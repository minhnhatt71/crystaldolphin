// Package config defines the configuration schema for crystaldolphin.
//
// JSON keys use camelCase to stay byte-compatible with existing
// ~/.nanobot/config.json files created by the Python nanobot.
package config

import (
	"os"
	"path/filepath"

	agentcfg "github.com/crystaldolphin/crystaldolphin/internal/config/agent"
	channelcfg "github.com/crystaldolphin/crystaldolphin/internal/config/channel"
	gatewaycfg "github.com/crystaldolphin/crystaldolphin/internal/config/gateway"
	providercfg "github.com/crystaldolphin/crystaldolphin/internal/config/provider"
	toolcfg "github.com/crystaldolphin/crystaldolphin/internal/config/tool"
)

// Config is the root configuration object, loaded from ~/.nanobot/config.json.
type Config struct {
	Agents    agentcfg.AgentsConfig       `json:"agents"`
	Channels  channelcfg.ChannelsConfig   `json:"channels"`
	Gateway   gatewaycfg.GatewayConfig    `json:"gateway"`
	Tools     toolcfg.ToolsConfig         `json:"tools"`
	Providers providercfg.ProvidersConfig `json:"providers"`
}

// DefaultConfig returns a Config populated with all default values.
func DefaultConfig() Config {
	return Config{
		Tools:     toolcfg.DefaultToolConfigs(),
		Agents:    agentcfg.DefaultAgentsConfig(),
		Gateway:   gatewaycfg.DefaultGatewayConfig(),
		Channels:  channelcfg.DefaultChannelsConfig(),
		Providers: providercfg.DefaultProvidersConfig(),
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
func (c *Config) ProviderByName(name string) *providercfg.ProviderConfig {
	return c.Providers.ByName(name)
}
