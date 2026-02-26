package dependency

import (
	"fmt"

	"go.uber.org/dig"

	"github.com/crystaldolphin/crystaldolphin/internal/agent"
	"github.com/crystaldolphin/crystaldolphin/internal/bus"
	"github.com/crystaldolphin/crystaldolphin/internal/config"
	"github.com/crystaldolphin/crystaldolphin/internal/cron"
	"github.com/crystaldolphin/crystaldolphin/internal/providers"
	"github.com/crystaldolphin/crystaldolphin/internal/schema"
	"github.com/crystaldolphin/crystaldolphin/internal/session"
	"github.com/crystaldolphin/crystaldolphin/internal/tools"
)

// ServiceContainer holds the resolved core service singletons.
// Callers use the typed getter methods; they never need to import dig directly.
type ServiceContainer struct {
	provider schema.LLMProvider
	msgBus   bus.Bus
	loop     schema.AgentLooper
	cronSvc  *cron.JobManager
}

func (c *ServiceContainer) Provider() schema.LLMProvider  { return c.provider }
func (c *ServiceContainer) MessageBus() bus.Bus           { return c.msgBus }
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
	if err := d.Provide(newMessageBus); err != nil {
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
	if err := d.Provide(newSubagentManager); err != nil {
		return nil, err
	}
	if err := d.Provide(newAgentRegistry); err != nil {
		return nil, err
	}
	if err := d.Provide(newMemoryStore); err != nil {
		return nil, err
	}
	if err := d.Provide(newConsolidator); err != nil {
		return nil, err
	}
	if err := d.Provide(newSkillsLoader); err != nil {
		return nil, err
	}
	if err := d.Provide(newContextBuilder); err != nil {
		return nil, err
	}
	if err := d.Provide(newAgentLoop); err != nil {
		return nil, err
	}

	var result *ServiceContainer
	err := d.Invoke(func(
		provider schema.LLMProvider,
		msgBus bus.Bus,
		loop schema.AgentLooper,
		cronSvc *cron.JobManager,
	) {
		result = &ServiceContainer{
			provider: provider,
			msgBus:   msgBus,
			loop:     loop,
			cronSvc:  cronSvc,
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

func newMessageBus() bus.Bus {
	return bus.NewMessageBus(100)
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
		WithTool(tools.NewReadFileTool(workspace, allowedDir)).
		WithTool(tools.NewWriteFileTool(workspace, allowedDir)).
		WithTool(tools.NewEditFileTool(workspace, allowedDir)).
		WithTool(tools.NewExecTool(workspace, cfg.Tools.Exec.Timeout, cfg.Tools.RestrictToWorkspace)).
		WithTool(tools.NewWebSearchTool(cfg.Tools.Web.Search.APIKey, cfg.Tools.Web.Search.MaxResults)).
		WithTool(tools.NewWebFetchTool(0)).
		Build()

	return SubagentRegistry{registry}
}

func newSubagentManager(p schema.LLMProvider, b bus.Bus, cfg *config.Config, m LLMModel, reg SubagentRegistry) *agent.SubagentManager {
	return agent.NewSubagentManager(
		p, cfg.WorkspacePath(), b,
		string(m),
		cfg.Agents.Defaults.Temperature,
		cfg.Agents.Defaults.MaxTokens,
		reg.Registry,
	)
}

func newAgentRegistry(
	cfg *config.Config,
	b bus.Bus,
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
		WithTool(tools.NewReadFileTool(workspace, allowedDir)).
		WithTool(tools.NewWriteFileTool(workspace, allowedDir)).
		WithTool(tools.NewEditFileTool(workspace, allowedDir)).
		WithTool(tools.NewListDirTool(workspace, allowedDir)).
		WithTool(tools.NewExecTool(workspace, cfg.Tools.Exec.Timeout, cfg.Tools.RestrictToWorkspace)).
		WithTool(tools.NewWebSearchTool(cfg.Tools.Web.Search.APIKey, cfg.Tools.Web.Search.MaxResults)).
		WithTool(tools.NewWebFetchTool(0)).
		WithTool(tools.NewMessageTool(b)).
		WithTool(tools.NewSpawnTool(subMgr)).
		WithTool(tools.NewCronTool(cronMgr)).
		WithTool(tools.NewSaveMemoryTool(mem)).
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

func newConsolidator(mem schema.MemoryStore, saver *session.Manager, p schema.LLMProvider, m LLMModel, reg AgentRegistry) schema.MemoryConsolidator {
	return agent.NewConsolidator(mem, saver, p, string(m), reg.Registry)
}

func newSkillsLoader(cfg *config.Config) schema.SkillLoader {
	return agent.NewSkillsLoader(cfg.WorkspacePath(), "")
}

func newContextBuilder(cfg *config.Config, mem schema.MemoryStore, sl schema.SkillLoader) *agent.AgentContextBuilder {
	return agent.NewContextBuilder(cfg.WorkspacePath(), mem, sl)
}

func newAgentLoop(
	b bus.Bus,
	p schema.LLMProvider,
	cfg *config.Config,
	sessions *session.Manager,
	mem schema.MemoryStore,
	consolidator schema.MemoryConsolidator,
	subMgr *agent.SubagentManager,
	reg AgentRegistry,
	cb *agent.AgentContextBuilder,
) schema.AgentLooper {
	return agent.NewAgentLoop(b, p, cfg, sessions, consolidator, reg.Registry, subMgr, cb)
}
