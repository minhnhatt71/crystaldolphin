package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/crystaldolphin/crystaldolphin/internal/schema"
	"github.com/crystaldolphin/crystaldolphin/internal/tools"
)

// MemoryCompactor orchestrates memory consolidation. It is responsible for
// selecting messages, calling the LLM, and persisting results via a MemoryStore.
// Storage I/O is delegated to the injected store; LLM interaction is done here.
type MemoryCompactor struct {
	reg         *tools.Registry
	memoryStore schema.MemoryStore
	saver       schema.SessionSaver
	provider    schema.LLMProvider
	model       string
}

// NewCompactor returns a MemoryCompactor. The save_memory tool is resolved
// from reg; if absent it falls back to constructing one directly from store.
func NewCompactor(store schema.MemoryStore, saver schema.SessionSaver, provider schema.LLMProvider, model string, reg *tools.Registry) *MemoryCompactor {
	registry := tools.NewRegistryBuilder().
		WithTool(tools.NewSaveMemoryTool(store)).
		Build()

	return &MemoryCompactor{
		saver:       saver,
		provider:    provider,
		model:       model,
		memoryStore: store,
		reg:         registry,
	}
}

// Consolidate summarises old session messages into MEMORY.md and HISTORY.md
// via a single LLM tool call. It is safe to call concurrently for different
// sessions; the caller must guard against concurrent calls for the same session
// (see AgentLoop.consolidating sync.Map).
//
// archive=true processes every message (used on /new); otherwise only the
// slice between LastConsolidated and len-keepCount is processed.
func (c *MemoryCompactor) Compact(ctx context.Context, s schema.Session, archiveAll bool, memoryWindow int) error {
	keepCount := memoryWindow / 2

	msgs, ok := s.ConsolidatedMessages(archiveAll, memoryWindow, keepCount)
	if !ok {
		return nil
	}

	if err := c.summarizeAndSave(ctx, msgs); err != nil {
		return err
	}

	s.Compact(archiveAll, keepCount)

	if err := c.saver.SaveConsolidated(s); err != nil {
		slog.Warn("memory consolidation: failed to persist session pointer", "err", err)
	}

	slog.Info("memory consolidation done", "last_consolidated", s.LastConsolidated())

	return nil
}

// summarizeAndSave sends oldMsgs to the LLM and invokes SaveMemoryTool.Execute
// with the returned arguments. Returns an error when the LLM call fails.
func (c *MemoryCompactor) summarizeAndSave(ctx context.Context, old schema.Messages) error {
	current := c.memoryStore.ReadLongTerm()
	if current == "" {
		current = "(empty)"
	}

	prompt := fmt.Sprintf(
		"Process this conversation and call the save_memory tool with your consolidation.\n\n"+
			"## Current Long-term Memory\n%s\n\n"+
			"## Conversation to Process\n%s",
		current,
		formatMessagesForPrompt(old.Messages),
	)

	messages := schema.NewMessages(
		schema.NewSystemMessage("You are a memory consolidation agent. Call the save_memory tool with your consolidation of the conversation."),
		schema.NewUserMessage(prompt),
	)

	err := c.reg.RunToolTurn(ctx, c.provider, messages, schema.NewChatOptions(c.model, 4096, 0.3))
	if err != nil {
		return fmt.Errorf("consolidation LLM call: %w", err)
	}

	return nil
}

// formatMessagesForPrompt renders a slice of messages into labelled text lines
// suitable for inclusion in the consolidation prompt.
func formatMessagesForPrompt(msgs []schema.Message) string {
	ts := time.Now().UTC().Format("2006-01-02T15:04")
	var lines []string
	for _, msg := range msgs {
		content := ""
		switch v := msg.Content.(type) {
		case string:
			content = v
		case *string:
			if v != nil {
				content = *v
			}
		}
		if content == "" {
			continue
		}
		toolsStr := ""
		if len(msg.ToolsUsed) > 0 {
			toolsStr = " [tools: " + strings.Join(msg.ToolsUsed, ", ") + "]"
		}
		lines = append(lines, fmt.Sprintf("[%s] %s%s: %s", ts, strings.ToUpper(string(msg.Role)), toolsStr, content))
	}

	return strings.Join(lines, "\n")
}
