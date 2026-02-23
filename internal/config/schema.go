// Package config defines the configuration schema for crystaldolphin.
//
// JSON keys use camelCase to stay byte-compatible with existing
// ~/.nanobot/config.json files created by the Python nanobot.
package config

import (
	"os"
	"path/filepath"

	"github.com/crystaldolphin/crystaldolphin/internal/config/agent"
	"github.com/crystaldolphin/crystaldolphin/internal/config/channel"
	"github.com/crystaldolphin/crystaldolphin/internal/config/gateway"
	"github.com/crystaldolphin/crystaldolphin/internal/config/provider"
	"github.com/crystaldolphin/crystaldolphin/internal/config/tool"
)

// Config is the root configuration object, loaded from ~/.nanobot/config.json.
type Config struct {
	Agents    agent.AgentsConfig       `json:"agents"`
	Channels  channel.ChannelsConfig   `json:"channels"`
	Gateway   gateway.GatewayConfig    `json:"gateway"`
	Tools     tool.ToolsConfig         `json:"tools"`
	Providers provider.ProvidersConfig `json:"providers"`
}

// DefaultConfig returns a Config populated with all default values.
func DefaultConfig() Config {
	return Config{
		Tools:     tool.DefaultToolConfigs(),
		Agents:    agent.DefaultAgentsConfig(),
		Gateway:   gateway.DefaultGatewayConfig(),
		Channels:  channel.DefaultChannelsConfig(),
		Providers: provider.DefaultProvidersConfig(),
	}
}

// WorkspacePath returns the expanded absolute path to the agent workspace.
func (c *Config) WorkspacePath() string {
	ws := c.Agents.Defaults.Workspace
	if ws == "" {
		ws = "~/.nanobot/workspace"
	}
	if len(ws) >= 2 && ws[:2] == "~/" {
		home, err := os.UserHomeDir()
		if err == nil {
			ws = filepath.Join(home, ws[2:])
		}
	}
	return ws
}

// ProviderByName returns a pointer to the ProviderConfig field matching the
// given registry name. Returns nil if unknown.
func (c *Config) ProviderByName(name string) *provider.ProviderConfig {
	return c.Providers.ByName(name)
}
