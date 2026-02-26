// Package dependency wires core crystaldolphin services using go.uber.org/dig.
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

// Container holds the resolved core service singletons.
// Callers use the typed getter methods; they never need to import dig directly.
type Container struct {
	provider schema.LLMProvider
	msgBus   *bus.MessageBus
	loop     *agent.AgentLoop
	cronSvc  *cron.JobManager
}

func (c *Container) Provider() schema.LLMProvider  { return c.provider }
func (c *Container) MessageBus() *bus.MessageBus   { return c.msgBus }
func (c *Container) AgentLoop() *agent.AgentLoop   { return c.loop }
func (c *Container) CronService() *cron.JobManager { return c.cronSvc }

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
func New(cfg *config.Config) (*Container, error) {
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
	if err := d.Provide(newContextBuilder); err != nil {
		return nil, err
	}
	if err := d.Provide(newAgentLoop); err != nil {
		return nil, err
	}

	var result *Container
	err := d.Invoke(func(
		provider schema.LLMProvider,
		msgBus *bus.MessageBus,
		loop *agent.AgentLoop,
		cronSvc *cron.JobManager,
	) {
		result = &Container{
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

func newMessageBus() *bus.MessageBus {
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

func newSubagentManager(p schema.LLMProvider, b *bus.MessageBus, cfg *config.Config, m LLMModel, reg SubagentRegistry) *agent.SubagentManager {
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
	b *bus.MessageBus,
	subMgr *agent.SubagentManager,
	cronMgr *cron.JobManager,
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
		Build()

	return AgentRegistry{registry}
}

func newContextBuilder(cfg *config.Config) *agent.ContextBuilder {
	return agent.NewContextBuilder(cfg.WorkspacePath(), "")
}

func newAgentLoop(
	b *bus.MessageBus,
	p schema.LLMProvider,
	cfg *config.Config,
	sessions *session.Manager,
	subMgr *agent.SubagentManager,
	reg AgentRegistry,
	cb *agent.ContextBuilder,
) *agent.AgentLoop {
	return agent.NewAgentLoop(b, p, cfg, sessions, reg.Registry, subMgr, cb)
}
