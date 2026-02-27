package agent

import (
	"context"

	"github.com/crystaldolphin/crystaldolphin/internal/mcp"
	"github.com/crystaldolphin/crystaldolphin/internal/schema"
	"github.com/crystaldolphin/crystaldolphin/internal/tools"
)

// CoreAgent processes a single user-facing request.
// It carries the full tool set (including spawn, message, cron) and uses the
// rich system prompt built from workspace files and memory.
// Constructed per message by AgentFactory.NewCoreAgent().
type CoreAgent struct {
	LoopRunner

	tools      *tools.ToolList // pointer to AgentLoop.tools — picks up MCP-added tools automatically
	mcpManager *mcp.Manager
}

// Execute implements schema.Agent.
// conversation must be fully built by the caller (system prompt + history + user message).
// It connects MCP servers on the first call (no-op on subsequent calls).
func (a *CoreAgent) Execute(ctx context.Context, conversation schema.Messages, onProgress func(string)) (string, []string) {
	a.mcpManager.ConnectOnce(ctx, a.tools)

	return a.run(ctx, conversation, a.tools, onProgress)
}
