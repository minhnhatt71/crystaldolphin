package tools

import (
	"context"
	"fmt"

	"github.com/crystaldolphin/crystaldolphin/internal/schema"
)

// ToolName is the canonical name of a built-in tool.
type ToolName string

const (
	ToolExec       ToolName = "exec"
	ToolReadFile   ToolName = "read_file"
	ToolWriteFile  ToolName = "write_file"
	ToolEditFile   ToolName = "edit_file"
	ToolListDir    ToolName = "list_dir"
	ToolWebSearch  ToolName = "web_search"
	ToolWebFetch   ToolName = "web_fetch"
	ToolMessage    ToolName = "message"
	ToolSpawn      ToolName = "spawn"
	ToolCron       ToolName = "cron"
	ToolSaveMemory ToolName = "save_memory"
)

// Registry holds a set of named tools and exposes them for execution.
type Registry struct {
	tools map[string]schema.Tool
}

// Get returns the tool with the given name, or nil.
func (r *Registry) Get(name ToolName) schema.Tool {
	return r.tools[string(name)]
}

func (r *Registry) GetAll() ToolList {
	list := ToolList{tools: make(map[string]schema.Tool, len(r.tools))}
	for k, t := range r.tools {
		list.tools[k] = t
	}
	return list
}

// RunToolTurn sends messages to the provider with this registry's tool
// definitions, then executes every tool call in the response.
// Returns an error if the Chat call fails or any tool execution fails.
// If the LLM returns no tool calls the function is a no-op (returns nil).
func (r *Registry) RunToolTurn(ctx context.Context, provider schema.LLMProvider, messages schema.Messages, opts schema.ChatOptions) error {
	all := r.GetAll()
	defs := all.Definitions()
	resp, err := provider.Chat(ctx, messages, defs, opts)
	if err != nil {
		return err
	}

	for _, call := range resp.ToolCalls {
		t := r.tools[call.Name]
		if t == nil {
			return fmt.Errorf("tool %q not found", call.Name)
		}
		if _, err := t.Execute(ctx, call.Arguments); err != nil {
			return err
		}
	}

	return nil
}
