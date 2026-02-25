// Package interfaces contains the core contracts shared across crystaldolphin packages.
// Concrete implementations live in their respective packages; this package is the
// single canonical source of truth for every interface definition.
package interfaces

import (
	"context"
	"encoding/json"
)

// Tool is the interface all LLM-callable tools must satisfy.
// Built-in tools and MCP-wrapped tools both implement this interface.
type Tool interface {
	Name() string
	Description() string
	// Parameters returns the JSON Schema (as raw JSON bytes) for this tool's parameters.
	Parameters() json.RawMessage
	Execute(ctx context.Context, params map[string]any) (string, error)
}

// ToolList is a pre-built slice of tool definitions in OpenAI function-calling
type ToolList struct {
	tools map[string]Tool
}

func NewToolList(tools []Tool) ToolList {
	list := ToolList{tools: make(map[string]Tool, len(tools))}
	for _, t := range tools {
		list.tools[t.Name()] = t
	}

	return list
}

func (r *ToolList) Get(name string) Tool { return r.tools[name] }
func (r *ToolList) Add(t Tool) Tool      { r.tools[t.Name()] = t; return t }

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
