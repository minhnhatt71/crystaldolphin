// Package agent contains the core agent loop and its supporting components.
package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/crystaldolphin/crystaldolphin/internal/schema"
	"github.com/crystaldolphin/crystaldolphin/internal/session"
)

// SessionSaver is the subset of session.Manager needed by Consolidate.
type SessionSaver interface {
	Save(s *session.Session) error
}

type MemoryStore interface {
	ReadLongTerm() string
	WriteLongTerm(content string) error
	AppendHistory(entry string) error
	GetMemoryContext() string
	Consolidate(ctx context.Context,
		s *session.Session, saver SessionSaver, provider schema.LLMProvider,
		model string, archiveAll bool, memoryWindow int,
	) error
}

type FileMemoryStore struct {
	memoryDir       string
	memoryFilePath  string
	historyFilePath string
}

// NewMemoryStore creates a MemoryStore rooted at workspace.
// The memory/ subdirectory is created if it does not exist.
func NewMemoryStore(workspace string) (*FileMemoryStore, error) {
	dir := filepath.Join(workspace, "memory")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create memory dir: %w", err)
	}

	return &FileMemoryStore{
		memoryDir:       dir,
		memoryFilePath:  filepath.Join(dir, "MEMORY.md"),
		historyFilePath: filepath.Join(dir, "HISTORY.md"),
	}, nil
}

// ReadLongTerm returns the current contents of MEMORY.md, or "" if not yet written.
func (m *FileMemoryStore) ReadLongTerm() string {
	data, err := os.ReadFile(m.memoryFilePath)
	if err != nil {
		return ""
	}
	return string(data)
}

// WriteLongTerm overwrites MEMORY.md with content.
func (m *FileMemoryStore) WriteLongTerm(content string) error {
	return os.WriteFile(m.memoryFilePath, []byte(content), 0o644)
}

// AppendHistory appends a timestamped entry to HISTORY.md followed by a blank line.
func (m *FileMemoryStore) AppendHistory(entry string) error {
	f, err := os.OpenFile(m.historyFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open history file: %w", err)
	}
	defer f.Close()

	// Strip trailing whitespace, add double newline (matches Python behaviour).
	line := entry
	for len(line) > 0 && (line[len(line)-1] == '\n' || line[len(line)-1] == '\r' || line[len(line)-1] == ' ') {
		line = line[:len(line)-1]
	}
	_, err = fmt.Fprintf(f, "%s\n\n", line)
	return err
}

// GetMemoryContext returns the long-term memory formatted for injection into
// the system prompt, or "" if MEMORY.md is empty.
func (m *FileMemoryStore) GetMemoryContext() string {
	lt := m.ReadLongTerm()
	if lt == "" {
		return ""
	}
	return "## Long-term Memory\n" + lt
}

// Consolidate summarises old session messages into MEMORY.md and HISTORY.md
// via a single LLM tool call. It is safe to call concurrently for different
// sessions; the caller must guard against concurrent calls for the same session
// (see AgentLoop.consolidating sync.Map).
//
// archiveAll=true processes every message (used on /new); otherwise only the
// slice between LastConsolidated and len-keepCount is processed.
func (m *FileMemoryStore) Consolidate(ctx context.Context,
	s *session.Session,
	saver SessionSaver,
	provider schema.LLMProvider,
	model string,
	archiveAll bool,
	memoryWindow int,
) error {
	s.Lock()
	x := s.Messages.Clone()
	lastConsolidated := s.LastConsolidated
	s.Unlock()

	var oldMessages []schema.Message
	var keepCount int

	msgs := x.Messages

	if archiveAll {
		oldMessages = msgs
		keepCount = 0
		slog.Info("memory consolidation (archive_all)", "messages", len(msgs))
	} else {
		keepCount = memoryWindow / 2
		if len(msgs) <= keepCount {
			return nil
		}
		if len(msgs)-lastConsolidated <= 0 {
			return nil
		}
		end := len(msgs) - keepCount
		if end <= lastConsolidated {
			return nil
		}
		oldMessages = msgs[lastConsolidated:end]
		if len(oldMessages) == 0 {
			return nil
		}
		slog.Info("memory consolidation", "to_consolidate", len(oldMessages), "keep", keepCount)
	}

	// Format messages for the LLM prompt.
	ts := time.Now().UTC().Format("2006-01-02T15:04")
	var lines []string
	for _, msg := range oldMessages {
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
		lines = append(lines, fmt.Sprintf("[%s] %s%s: %s", ts, upper(msg.Role), toolsStr, content))
	}

	currentMemory := m.ReadLongTerm()
	prompt := fmt.Sprintf(
		"Process this conversation and call the save_memory tool with your consolidation.\n\n"+
			"## Current Long-term Memory\n%s\n\n"+
			"## Conversation to Process\n%s",
		orEmpty(currentMemory, "(empty)"),
		strings.Join(lines, "\n"),
	)

	messages := schema.NewMessages(
		schema.NewSystemMessage("You are a memory consolidation agent. Call the save_memory tool with your consolidation of the conversation."),
		schema.NewUserMessage(prompt),
	)

	resp, err := provider.Chat(ctx,
		messages,
		saveMemoryTool,
		schema.ChatOptions{
			Model:       model,
			MaxTokens:   4096,
			Temperature: 0.3,
		},
	)
	if err != nil {
		return fmt.Errorf("consolidation LLM call: %w", err)
	}

	if !resp.HasToolCalls() {
		slog.Warn("memory consolidation: LLM did not call save_memory, skipping")
		return nil
	}

	args := resp.ToolCalls[0].Arguments

	if entry := stringOrJSON(args["history_entry"]); entry != "" {
		if err := m.AppendHistory(entry); err != nil {
			slog.Warn("failed to append history", "err", err)
		}
	}
	if update := stringOrJSON(args["memory_update"]); update != "" {
		if update != currentMemory {
			if err := m.WriteLongTerm(update); err != nil {
				slog.Warn("failed to write long-term memory", "err", err)
			}
		}
	}

	// Advance the consolidation pointer and compact the in-memory slice.
	// Use len(msgs) from the cloned snapshot taken before the LLM call,
	// not s.Messages.Messages which may have grown concurrently.
	if archiveAll {
		s.Lock()
		s.LastConsolidated = 0
		s.Unlock()
	} else {
		// Compact drops already-consolidated messages and resets LastConsolidated
		// to 0 (the tail is now the start of the slice).
		s.Compact(keepCount)
	}

	// Persist the updated pointer immediately so it survives a restart.
	if err := saver.Save(s); err != nil {
		slog.Warn("memory consolidation: failed to persist session pointer", "err", err)
	}

	slog.Info("memory consolidation done", "messages", len(msgs), "last_consolidated", s.LastConsolidated)

	return nil
}
