package agent

import (
	"github.com/crystaldolphin/crystaldolphin/internal/mcp"
	"github.com/crystaldolphin/crystaldolphin/internal/schema"
	"github.com/crystaldolphin/crystaldolphin/internal/tools"
)

// AgentFactory creates per-request CoreAgent and SubAgent instances.
// It holds construction-time dependencies; created agents are lightweight
// objects that own only what they need for one execution.
type AgentFactory struct {
	provider    schema.LLMProvider
	settings    schema.AgentSettings // CoreAgent: full settings (maxIter=20, memoryWindow=N)
	subSettings schema.AgentSettings // SubAgent: restricted settings (maxIter=15, memoryWindow=0)
	coreTools   *tools.ToolList      // pointer to AgentLoop.tools — wired via SetCoreTools
	subTools    tools.ToolList       // value copy of restricted registry — no MCP tools
	mcpManager  *mcp.Manager
	workspace   string
}

// NewFactory constructs an AgentFactory.
// subRegistry is the restricted tool registry for SubAgents.
// The core ToolList is wired after AgentLoop construction via SetCoreTools.
func NewFactory(
	provider schema.LLMProvider,
	settings, subSettings schema.AgentSettings,
	subRegistry *tools.Registry,
	mcpManager *mcp.Manager,
	workspace string,
) *AgentFactory {
	return &AgentFactory{
		provider:    provider,
		settings:    settings,
		subSettings: subSettings,
		subTools:    subRegistry.GetAll(),
		mcpManager:  mcpManager,
		workspace:   workspace,
	}
}

// Close shuts down MCP server subprocesses. Called by AgentLoop.Run on exit.
func (f *AgentFactory) Close() {
	f.mcpManager.Close()
}

// SetCoreTools wires the factory to the AgentLoop's live ToolList.
// Must be called by NewAgentLoop before any CoreAgent is created.
// The pointer ensures MCP tools added via ConnectOnce are visible to all CoreAgents.
func (f *AgentFactory) SetCoreTools(tls *tools.ToolList) {
	f.coreTools = tls
}

// NewCoreAgent creates a CoreAgent ready to execute one user message.
func (f *AgentFactory) NewCoreAgent() *CoreAgent {
	return &CoreAgent{
		LoopRunner: newLoopRunner(f.provider, f.settings),
		tools:      f.coreTools,
		mcpManager: f.mcpManager,
	}
}

// NewSubAgent creates a SubAgent ready to execute one background task.
func (f *AgentFactory) NewSubAgent() *SubAgent {
	return &SubAgent{
		LoopRunner: newLoopRunner(f.provider, f.subSettings),
		tools:      f.subTools,
		workspace:  f.workspace,
	}
}
