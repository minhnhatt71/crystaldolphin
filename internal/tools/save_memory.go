package tools

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/crystaldolphin/crystaldolphin/internal/schema"
)

type SaveMemoryTool struct {
	store schema.MemoryStore
}

// NewSaveMemoryTool creates a SaveMemoryTool backed by the given MemoryStore.
func NewSaveMemoryTool(store schema.MemoryStore) *SaveMemoryTool {
	return &SaveMemoryTool{store: store}
}

func (t *SaveMemoryTool) Name() string { return "save_memory" }
func (t *SaveMemoryTool) Description() string {
	return "Save the memory consolidation result to persistent storage."
}

func (t *SaveMemoryTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"history_entry": {
				"type": "string",
				"description": "A paragraph (2-5 sentences) summarizing key events/decisions/topics. Start with [YYYY-MM-DD HH:MM]. Include detail useful for grep search."
			},
			"memory_update": {
				"type": "string",
				"description": "Full updated long-term memory as markdown. Include all existing facts plus new ones. Return unchanged if nothing new."
			}
		},
		"required": ["history_entry", "memory_update"]
	}`)
}

// Save writes the history entry and long-term memory returned by the LLM.
// current is the snapshot read before the LLM call; the memory_update
// write is skipped when the LLM returns it unchanged.
// Errors are logged but do not propagate.
func (t *SaveMemoryTool) Save(_ context.Context, args map[string]any, current string) (string, error) {
	if entry, _ := args["history_entry"].(string); entry != "" {
		if err := t.store.AppendHistory(entry); err != nil {
			slog.Warn("failed to append history", "err", err)
		}
	}
	if update, _ := args["memory_update"].(string); update != "" && update != current {
		if err := t.store.WriteLongTerm(update); err != nil {
			slog.Warn("failed to write long-term memory", "err", err)
		}
	}
	return "memory saved", nil
}

// Execute implements schema.Tool. It reads the current long-term memory from
// the store so it can skip a no-op write, then delegates to Save.
func (t *SaveMemoryTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	return t.Save(ctx, args, t.store.ReadLongTerm())
}
