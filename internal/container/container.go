// Package container wires core crystaldolphin services using go.uber.org/dig.
package container

import (
	"fmt"

	"go.uber.org/dig"

	"github.com/crystaldolphin/crystaldolphin/internal/agent"
	"github.com/crystaldolphin/crystaldolphin/internal/bus"
	"github.com/crystaldolphin/crystaldolphin/internal/config"
	"github.com/crystaldolphin/crystaldolphin/internal/cron"
	"github.com/crystaldolphin/crystaldolphin/internal/providers"
)

// Container holds the resolved core service singletons.
// Callers use the typed getter methods; they never need to import dig directly.
type Container struct {
	provider providers.LLMProvider
	msgBus   *bus.MessageBus
	loop     *agent.AgentLoop
	cronSvc  *cron.JobManager
}

func (c *Container) Provider() providers.LLMProvider { return c.provider }
func (c *Container) MessageBus() *bus.MessageBus     { return c.msgBus }
func (c *Container) AgentLoop() *agent.AgentLoop     { return c.loop }
func (c *Container) CronService() *cron.JobManager   { return c.cronSvc }

// New builds and wires all core services from cfg.
// The cron ↔ agent two-step wiring (SetCronTool) is performed internally.
func New(cfg *config.Config) (*Container, error) {
	d := dig.New()

	if err := d.Provide(func() *config.Config { return cfg }); err != nil {
		return nil, err
	}
	if err := d.Provide(newProvider); err != nil {
		return nil, err
	}
	if err := d.Provide(newMessageBus); err != nil {
		return nil, err
	}
	if err := d.Provide(newCronService); err != nil {
		return nil, err
	}
	if err := d.Provide(newAgentLoop); err != nil {
		return nil, err
	}

	var result *Container
	err := d.Invoke(func(
		provider providers.LLMProvider,
		msgBus *bus.MessageBus,
		loop *agent.AgentLoop,
		cronSvc *cron.JobManager,
	) {
		loop.SetCronTool(cronSvc)
		result = &Container{
			provider: provider,
			msgBus:   msgBus,
			loop:     loop,
			cronSvc:  cronSvc,
		}
	})
	return result, err
}

func newProvider(cfg *config.Config) (providers.LLMProvider, error) {
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

func newCronService(cfg *config.Config) *cron.JobManager {
	cronPath := config.DataDir() + "/cron/jobs.json"
	_ = cfg // reserved for future per-config cron settings
	return cron.NewService(cronPath)
}

func newAgentLoop(b *bus.MessageBus, p providers.LLMProvider, cfg *config.Config) *agent.AgentLoop {
	return agent.NewAgentLoop(b, p, cfg, "")
}
