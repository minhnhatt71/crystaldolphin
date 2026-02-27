// Package interfaces contains the core contracts shared across crystaldolphin packages.
// Concrete implementations live in their respective packages; this package is the
// single canonical source of truth for every interface definition.
package schema

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

type ToolRegistry interface {
	// Get returns the tool with the given name, or nil if not found.
	Get(name string) Tool
	// Add registers a new tool, replacing any existing tool with the same name.
	Add(t Tool) Tool
	// Definitions returns all tool definitions in OpenAI function-calling format.
	Definitions() []map[string]any
}

// ToolRegistrar is implemented by any collection that accepts Tool registrations
// at runtime. Defined here so packages like internal/mcp can register discovered
// tools without importing internal/tools.
type ToolRegistrar interface {
	Add(t Tool) Tool
}
