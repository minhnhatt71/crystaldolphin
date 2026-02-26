package tools

import (
	"encoding/json"

	"github.com/crystaldolphin/crystaldolphin/internal/schema"
)

// ToolList holds a named set of tools and exposes them for LLM calls and
// runtime extension (e.g. MCP servers).
type ToolList struct {
	tools map[string]schema.Tool
}

func NewToolList(ts ...schema.Tool) *ToolList {
	list := ToolList{tools: make(map[string]schema.Tool, len(ts))}
	for _, t := range ts {
		list.tools[t.Name()] = t
	}

	return &list
}

// Get returns the tool with the given name, or nil if not found.
func (r *ToolList) Get(name string) schema.Tool {
	return r.tools[name]
}

// Add registers a new tool, replacing any existing tool with the same name.
func (r *ToolList) Add(t schema.Tool) schema.Tool {
	r.tools[t.Name()] = t

	return t
}

// Definitions returns all tool definitions in OpenAI function-calling format.
func (r *ToolList) Definitions() []map[string]any {
	list := make([]map[string]any, 0, len(r.tools))
	for _, t := range r.tools {
		var params any
		if err := json.Unmarshal(t.Parameters(), &params); err != nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		list = append(list, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name(),
				"description": t.Description(),
				"parameters":  params,
			},
		})
	}
	return list
}
