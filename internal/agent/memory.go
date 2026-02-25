// Package agent contains the core agent loop and its supporting components.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/crystaldolphin/crystaldolphin/internal/providers"
	"github.com/crystaldolphin/crystaldolphin/internal/session"
)

// saveMemoryTool is the OpenAI function definition sent to the LLM during
// consolidation. Must be byte-identical to nanobot's Python _SAVE_MEMORY_TOOL
// so the same model prompting works without retuning.
var saveMemoryTool = []map[string]any{
	{
		"type": "function",
		"function": map[string]any{
			"name":        "save_memory",
			"description": "Save the memory consolidation result to persistent storage.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"history_entry": map[string]any{
						"type": "string",
						"description": "A paragraph (2-5 sentences) summarizing key events/decisions/topics. " +
							"Start with [YYYY-MM-DD HH:MM]. Include detail useful for grep search.",
					},
					"memory_update": map[string]any{
						"type": "string",
						"description": "Full updated long-term memory as markdown. Include all existing " +
							"facts plus new ones. Return unchanged if nothing new.",
					},
				},
				"required": []string{"history_entry", "memory_update"},
			},
		},
	},
}

// MemoryStore manages two persistent memory files in the workspace:
//   - memory/MEMORY.md — long-term facts, overwritten on each consolidation
//   - memory/HISTORY.md — append-only event log
type MemoryStore struct {
	memoryDir   string
	memoryFile  string
	historyFile string
}

// NewMemoryStore creates a MemoryStore rooted at workspace.
// The memory/ subdirectory is created if it does not exist.
func NewMemoryStore(workspace string) (*MemoryStore, error) {
	dir := filepath.Join(workspace, "memory")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create memory dir: %w", err)
	}
	return &MemoryStore{
		memoryDir:   dir,
		memoryFile:  filepath.Join(dir, "MEMORY.md"),
		historyFile: filepath.Join(dir, "HISTORY.md"),
	}, nil
}

// ReadLongTerm returns the current contents of MEMORY.md, or "" if not yet written.
func (m *MemoryStore) ReadLongTerm() string {
	data, err := os.ReadFile(m.memoryFile)
	if err != nil {
		return ""
	}
	return string(data)
}

// WriteLongTerm overwrites MEMORY.md with content.
func (m *MemoryStore) WriteLongTerm(content string) error {
	return os.WriteFile(m.memoryFile, []byte(content), 0o644)
}

// AppendHistory appends a timestamped entry to HISTORY.md followed by a blank line.
func (m *MemoryStore) AppendHistory(entry string) error {
	f, err := os.OpenFile(m.historyFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
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
func (m *MemoryStore) GetMemoryContext() string {
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
func (m *MemoryStore) Consolidate(
	ctx context.Context,
	s *session.Session,
	provider providers.LLMProvider,
	model string,
	archiveAll bool,
	memoryWindow int,
) error {
	s.Lock()
	msgs := make([]map[string]any, len(s.Messages))
	copy(msgs, s.Messages)
	lastConsolidated := s.LastConsolidated
	s.Unlock()

	var oldMessages []map[string]any
	var keepCount int

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
	var lines []string
	for _, msg := range oldMessages {
		content, _ := msg["content"].(string)
		if content == "" {
			continue
		}
		ts, _ := msg["timestamp"].(string)
		if len(ts) > 16 {
			ts = ts[:16]
		}
		role, _ := msg["role"].(string)
		tools := ""
		if tu, ok := msg["tools_used"].([]any); ok && len(tu) > 0 {
			names := make([]string, 0, len(tu))
			for _, t := range tu {
				if s, ok := t.(string); ok {
					names = append(names, s)
				}
			}
			if len(names) > 0 {
				tools = " [tools: " + joinStrings(names, ", ") + "]"
			}
		}
		lines = append(lines, fmt.Sprintf("[%s] %s%s: %s", ts, upper(role), tools, content))
	}

	currentMemory := m.ReadLongTerm()
	prompt := fmt.Sprintf(
		"Process this conversation and call the save_memory tool with your consolidation.\n\n"+
			"## Current Long-term Memory\n%s\n\n"+
			"## Conversation to Process\n%s",
		orEmpty(currentMemory, "(empty)"),
		joinStrings(lines, "\n"),
	)

	messages := NewMessageHistory()
	messages.AddSystem("You are a memory consolidation agent. Call the save_memory tool with your consolidation of the conversation.")
	messages.AddUser(prompt)

	resp, err := provider.Chat(ctx,
		messages,
		saveMemoryTool,
		providers.ChatOptions{
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

	// Advance the consolidation pointer.
	s.Lock()
	if archiveAll {
		s.LastConsolidated = 0
	} else {
		s.LastConsolidated = len(s.Messages) - keepCount
	}
	s.Unlock()

	slog.Info("memory consolidation done", "messages", len(msgs), "last_consolidated", s.LastConsolidated)

	return nil
}

// ---------------------------------------------------------------------------
// Small helpers

func upper(s string) string {
	if s == "" {
		return s
	}
	b := []byte(s)
	if b[0] >= 'a' && b[0] <= 'z' {
		b[0] -= 32
	}
	return string(b)
}

func orEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func joinStrings(ss []string, sep string) string {
	var b strings.Builder
	for i, s := range ss {
		if i > 0 {
			b.WriteString(sep)
		}
		b.WriteString(s)
	}
	return b.String()
}

// stringOrJSON coerces a value from the tool arguments to a string.
// If it's already a string, return it. Otherwise JSON-encode it.
func stringOrJSON(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}
