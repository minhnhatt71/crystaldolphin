package dependency

import (
	"fmt"

	"go.uber.org/dig"

	"github.com/crystaldolphin/crystaldolphin/internal/agent"
	"github.com/crystaldolphin/crystaldolphin/internal/bus"
	"github.com/crystaldolphin/crystaldolphin/internal/config"
	"github.com/crystaldolphin/crystaldolphin/internal/cron"
	"github.com/crystaldolphin/crystaldolphin/internal/mcp"
	"github.com/crystaldolphin/crystaldolphin/internal/providers"
	"github.com/crystaldolphin/crystaldolphin/internal/schema"
	"github.com/crystaldolphin/crystaldolphin/internal/session"
	"github.com/crystaldolphin/crystaldolphin/internal/tools"
)

// ServiceContainer holds the resolved core service singletons.
// Callers use the typed getter methods; they never need to import dig directly.
type ServiceContainer struct {
	provider    schema.LLMProvider
	inboundBus  *bus.AgentBus
	outboundBus *bus.ChannelBus
	consoleBus  *bus.ConsoleBus
	loop        schema.AgentLooper
	cronSvc     *cron.JobManager
}

func (c *ServiceContainer) Provider() schema.LLMProvider  { return c.provider }
func (c *ServiceContainer) AgentBus() *bus.AgentBus       { return c.inboundBus }
func (c *ServiceContainer) ChannelBus() *bus.ChannelBus   { return c.outboundBus }
func (c *ServiceContainer) ConsoleBus() *bus.ConsoleBus   { return c.consoleBus }
func (c *ServiceContainer) AgentLoop() schema.AgentLooper { return c.loop }
func (c *ServiceContainer) CronService() *cron.JobManager { return c.cronSvc }

// LLMModel is a named string type so dig can distinguish it from plain
// strings when injecting the effective model name into providers that need it.
type LLMModel string

// AgentRegistry wraps the full tool registry used by the main agent loop.
type AgentRegistry struct{ *tools.Registry }

// SubagentRegistry wraps the restricted tool registry used by subagents.
// It must not contain spawn or message tools to prevent recursion and
// unsolicited outbound messages.
type SubagentRegistry struct{ *tools.Registry }

// New builds and wires all core services from cfg.
func New(cfg *config.Config) (*ServiceContainer, error) {
	d := dig.New()

	if err := d.Provide(func() *config.Config { return cfg }); err != nil {
		return nil, err
	}
	if err := d.Provide(newProvider); err != nil {
		return nil, err
	}
	if err := d.Provide(resolveLLMModel); err != nil {
		return nil, err
	}
	if err := d.Provide(newAgentBus); err != nil {
		return nil, err
	}
	if err := d.Provide(newChannelBus); err != nil {
		return nil, err
	}
	if err := d.Provide(newConsoleBus); err != nil {
		return nil, err
	}
	if err := d.Provide(newSessionManager); err != nil {
		return nil, err
	}
	if err := d.Provide(newCronService); err != nil {
		return nil, err
	}
	if err := d.Provide(newSubAgentToolRegistry); err != nil {
		return nil, err
	}
	if err := d.Provide(newAgentRegistry); err != nil {
		return nil, err
	}
	if err := d.Provide(newAgentFactory); err != nil {
		return nil, err
	}
	if err := d.Provide(newSubagentManager); err != nil {
		return nil, err
	}
	if err := d.Provide(newMemoryStore); err != nil {
		return nil, err
	}
	if err := d.Provide(newCompactor); err != nil {
		return nil, err
	}
	if err := d.Provide(newSkillsLoader); err != nil {
		return nil, err
	}
	if err := d.Provide(newContextBuilder); err != nil {
		return nil, err
	}
	if err := d.Provide(newMCPManager); err != nil {
		return nil, err
	}
	if err := d.Provide(newAgentLoop); err != nil {
		return nil, err
	}

	var result *ServiceContainer
	err := d.Invoke(func(
		provider schema.LLMProvider,
		inbound *bus.AgentBus,
		outbound *bus.ChannelBus,
		console *bus.ConsoleBus,
		loop schema.AgentLooper,
		cronSvc *cron.JobManager,
	) {
		result = &ServiceContainer{
			provider:    provider,
			inboundBus:  inbound,
			outboundBus: outbound,
			consoleBus:  console,
			loop:        loop,
			cronSvc:     cronSvc,
		}
	})
	return result, err
}

func newProvider(cfg *config.Config) (schema.LLMProvider, error) {
	model := cfg.Agents.Defaults.Model
	result := cfg.MatchProvider(model)

	if result.Provider == nil && !isOAuthProvider(result.Name) {
		return nil, fmt.Errorf("no API key configured for model %q — edit %s", model, config.ConfigPath())
	}

	apiKey := ""
	apiBase := ""
	var extraHeaders map[string]string
	if result.Provider != nil {
		apiKey = result.Provider.APIKey
		apiBase = result.Provider.APIBase
		extraHeaders = result.Provider.ExtraHeaders
	}
	if apiBase == "" {
		apiBase = cfg.GetAPIBase(model)
	}
	return providers.New(providers.Params{
		APIKey:       apiKey,
		APIBase:      apiBase,
		ExtraHeaders: extraHeaders,
		DefaultModel: model,
		ProviderName: result.Name,
	}), nil
}

func isOAuthProvider(name string) bool {
	spec := providers.FindByName(name)
	return spec != nil && spec.IsOAuth
}

func newAgentBus() *bus.AgentBus {
	return bus.NewAgentBus(100)
}

func newChannelBus() *bus.ChannelBus {
	return bus.NewChannelBus(100)
}

func newConsoleBus() *bus.ConsoleBus {
	return bus.NewConsoleBus(100)
}

func newSessionManager(cfg *config.Config) (*session.Manager, error) {
	return session.NewManager(cfg.WorkspacePath())
}

func newCronService(cfg *config.Config) *cron.JobManager {
	cronPath := config.DataDir() + "/cron/jobs.json"
	_ = cfg // reserved for future per-config cron settings
	return cron.NewService(cronPath)
}

func resolveLLMModel(cfg *config.Config, p schema.LLMProvider) LLMModel {
	m := cfg.Agents.Defaults.Model
	if m == "" {
		m = p.DefaultModel()
	}

	return LLMModel(m)
}

func newSubAgentToolRegistry(cfg *config.Config) SubagentRegistry {
	workspace := cfg.WorkspacePath()
	allowedDir := ""
	if cfg.Tools.RestrictToWorkspace {
		allowedDir = workspace
	}

	registry := tools.NewRegistryBuilder().
		Tool(tools.NewReadFileTool(workspace, allowedDir)).
		Tool(tools.NewWriteFileTool(workspace, allowedDir)).
		Tool(tools.NewEditFileTool(workspace, allowedDir)).
		Tool(tools.NewExecTool(workspace, cfg.Tools.Exec.Timeout, cfg.Tools.RestrictToWorkspace)).
		Tool(tools.NewWebSearchTool(cfg.Tools.Web.Search.APIKey, cfg.Tools.Web.Search.MaxResults)).
		Tool(tools.NewWebFetchTool(0)).
		Build()

	return SubagentRegistry{registry}
}

func newAgentFactory(
	p schema.LLMProvider,
	cfg *config.Config,
	m LLMModel,
	subReg SubagentRegistry,
	mcpMgr *mcp.Manager,
) *agent.AgentFactory {
	coreSettings := schema.NewAgentSettings(
		string(m),
		cfg.Agents.Defaults.MaxToolIter,
		cfg.Agents.Defaults.Temperature,
		cfg.Agents.Defaults.MaxTokens,
		cfg.Agents.Defaults.MemoryWindow,
	)

	subSettings := schema.NewAgentSettings(
		string(m),
		15,
		cfg.Agents.Defaults.Temperature,
		cfg.Agents.Defaults.MaxTokens,
		0,
	)

	return agent.NewFactory(p, coreSettings, subSettings, subReg.Registry, mcpMgr, cfg.WorkspacePath())
}

func newSubagentManager(factory *agent.AgentFactory, inbound *bus.AgentBus) *agent.SubagentManager {
	return agent.NewSubagentManager(factory, inbound)
}

func newAgentRegistry(
	cfg *config.Config,
	outbound *bus.ChannelBus,
	subMgr *agent.SubagentManager,
	cronMgr *cron.JobManager,
	mem schema.MemoryStore,
) AgentRegistry {
	workspace := cfg.WorkspacePath()
	allowedDir := ""
	if cfg.Tools.RestrictToWorkspace {
		allowedDir = workspace
	}

	registry := tools.NewRegistryBuilder().
		Tool(tools.NewReadFileTool(workspace, allowedDir)).
		Tool(tools.NewWriteFileTool(workspace, allowedDir)).
		Tool(tools.NewEditFileTool(workspace, allowedDir)).
		Tool(tools.NewListDirTool(workspace, allowedDir)).
		Tool(tools.NewExecTool(workspace, cfg.Tools.Exec.Timeout, cfg.Tools.RestrictToWorkspace)).
		Tool(tools.NewWebSearchTool(cfg.Tools.Web.Search.APIKey, cfg.Tools.Web.Search.MaxResults)).
		Tool(tools.NewWebFetchTool(0)).
		Tool(tools.NewMessageTool(outbound)).
		Tool(tools.NewSpawnTool(subMgr)).
		Tool(tools.NewCronTool(cronMgr)).
		Tool(tools.NewSaveMemoryTool(mem)).
		Build()

	return AgentRegistry{registry}
}

func newMemoryStore(cfg *config.Config) (schema.MemoryStore, error) {
	mem, err := agent.NewMemoryStore(cfg.WorkspacePath())
	if err != nil || mem == nil {
		return &agent.FileMemoryStore{}, nil
	}
	return mem, nil
}

func newCompactor(cfg *config.Config, mem schema.MemoryStore, saver *session.Manager, p schema.LLMProvider, m LLMModel, reg AgentRegistry) schema.MemoryCompactor {
	return agent.NewCompactor(mem, saver, p, string(m), cfg.Agents.Defaults.MemoryWindow, reg.Registry)
}

func newSkillsLoader(cfg *config.Config) schema.SkillLoader {
	return agent.NewSkillsLoader(cfg.WorkspacePath(), "")
}

func newContextBuilder(cfg *config.Config, mem schema.MemoryStore, sl schema.SkillLoader) *agent.PromptContext {
	return agent.NewContextBuilder(cfg.WorkspacePath(), mem, sl)
}

func newMCPManager(cfg *config.Config) *mcp.Manager {
	return mcp.NewManager(cfg.Tools.MCPServers)
}

func newAgentLoop(
	inbound *bus.AgentBus,
	outbound *bus.ChannelBus,
	factory *agent.AgentFactory,
	cfg *config.Config,
	m LLMModel,
	sessions *session.Manager,
	consolidator schema.MemoryCompactor,
	subMgr *agent.SubagentManager,
	reg AgentRegistry,
	cb *agent.PromptContext,
) schema.AgentLooper {
	settings := schema.NewAgentSettings(
		string(m),
		cfg.Agents.Defaults.MaxToolIter,
		cfg.Agents.Defaults.Temperature,
		cfg.Agents.Defaults.MaxTokens,
		cfg.Agents.Defaults.MemoryWindow,
	)

	return agent.NewAgentLoop(inbound, outbound, factory, settings, sessions, consolidator, reg.Registry, subMgr, cb)
}
