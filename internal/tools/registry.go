package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// Tool is the interface all built-in and MCP-wrapped tools must satisfy.
type Tool interface {
	Name() string
	Description() string
	// Parameters returns the JSON Schema (as raw JSON bytes) for this tool's parameters.
	Parameters() json.RawMessage
	Execute(ctx context.Context, params map[string]any) (string, error)
}

// Registry holds a set of named tools and exposes them for execution.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool to the registry.
func (r *Registry) Register(t Tool) {
	r.tools[t.Name()] = t
}

// Unregister removes a tool by name (no-op if not found).
func (r *Registry) Unregister(name string) {
	delete(r.tools, name)
}

// Has reports whether a tool with the given name is registered.
func (r *Registry) Has(name string) bool {
	_, ok := r.tools[name]
	return ok
}

// Get returns the tool with the given name, or nil.
func (r *Registry) Get(name string) Tool {
	return r.tools[name]
}

// Names returns all registered tool names.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.tools))
	for n := range r.tools {
		names = append(names, n)
	}
	return names
}

// GetDefinitions returns all tool definitions in OpenAI function-calling format.
func (r *Registry) GetDefinitions() []map[string]any {
	defs := make([]map[string]any, 0, len(r.tools))
	for _, t := range r.tools {
		var params any
		if err := json.Unmarshal(t.Parameters(), &params); err != nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		defs = append(defs, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name(),
				"description": t.Description(),
				"parameters":  params,
			},
		})
	}
	return defs
}

// Execute runs a named tool and returns its output as a string.
//
// Returns an error string (not a Go error) if the tool is missing or panics,
// matching Python's behaviour of returning error messages as strings.
func (r *Registry) Execute(ctx context.Context, name string, params map[string]any) string {
	t, ok := r.tools[name]
	if !ok {
		return fmt.Sprintf("Error: Tool '%s' not found", name)
	}
	result, err := t.Execute(ctx, params)
	if err != nil {
		return fmt.Sprintf("Error executing %s: %s", name, err)
	}
	return result
}
