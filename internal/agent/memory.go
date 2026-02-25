// Package agent contains the core agent loop and its supporting components.
package agent

import (
	"encoding/json"
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
