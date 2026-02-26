package tools

import (
	"github.com/crystaldolphin/crystaldolphin/internal/schema"
)

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
type Registry struct {
	tools map[string]schema.Tool
}

// GetTool returns the tool with the given name, or nil.
func (r *Registry) GetTool(name ToolName) schema.Tool {
	return r.tools[string(name)]
}

func (r *Registry) AllTools() ToolList {
	list := ToolList{tools: make(map[string]schema.Tool, len(r.tools))}
	for k, t := range r.tools {
		list.tools[k] = t
	}
	return list
}
