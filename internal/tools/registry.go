package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/crystaldolphin/crystaldolphin/internal/interfaces"
)

// Tool is the interface all built-in and MCP-wrapped tools must satisfy.
// The canonical definition lives in internal/interfaces; this alias keeps
// existing code compiling without changes.
type Tool = interfaces.Tool

// ToolName is the canonical name of a built-in tool.
type ToolName string

const (
	ToolExec      ToolName = "exec"
	ToolReadFile  ToolName = "read_file"
	ToolWriteFile ToolName = "write_file"
	ToolEditFile  ToolName = "edit_file"
	ToolListDir   ToolName = "list_dir"
	ToolWebSearch ToolName = "web_search"
	ToolWebFetch  ToolName = "web_fetch"
	ToolMessage   ToolName = "message"
	ToolSpawn     ToolName = "spawn"
	ToolCron      ToolName = "cron"
)

// Registry holds a set of named tools and exposes them for execution.
// Construct one via NewRegistryBuilder().WithTool(...).Build().
// After construction, MCP tools may be added at runtime via Add().
type Registry struct {
	tools map[string]Tool
}

// Add inserts a tool into an already-built Registry. Intended for runtime
// extension points such as MCP server connections that are established after
// initial construction.
func (r *Registry) Add(tool Tool) {
	r.tools[tool.Name()] = tool
}

// Has reports whether a tool with the given name is registered.
func (r *Registry) Has(name string) bool {
	_, ok := r.tools[name]
	return ok
}

// Get returns the tool with the given name, or nil.
func (r *Registry) Get(name ToolName) Tool {
	return r.tools[string(name)]
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
