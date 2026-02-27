package mcp

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	toolcfg "github.com/crystaldolphin/crystaldolphin/internal/config/tool"
	"github.com/crystaldolphin/crystaldolphin/internal/schema"
)

// Manager owns the lifecycle of all MCP server connections for a single agent.
type Manager struct {
	servers map[string]toolcfg.MCPServerConfig
	clients []*client
	once    sync.Once
}

// NewManager returns a Manager configured with the given MCP servers.
func NewManager(servers map[string]toolcfg.MCPServerConfig) *Manager {
	return &Manager{servers: servers}
}

// ConnectOnce connects to all configured MCP servers and registers their
// discovered tools into ts. It is safe to call concurrently; connection happens
// at most once. Failed servers are logged and skipped (non-fatal).
func (m *Manager) ConnectOnce(ctx context.Context, ts schema.ToolRegistrar) {
	m.once.Do(func() {
		for name, cfg := range m.servers {
			c := newClient(name, toServerConfig(cfg))
			if err := c.connect(ctx); err != nil {
				slog.Error("MCP server connect failed", "server", name, "err", err)
				continue
			}

			toolDefs, err := c.listTools(ctx)
			if err != nil {
				slog.Error("MCP server list_tools failed", "server", name, "err", err)
				continue
			}

			for _, toolDef := range toolDefs {
				toolName, _ := toolDef["name"].(string)
				if toolName == "" {
					continue
				}
				desc, _ := toolDef["description"].(string)
				inputSchema, _ := toolDef["inputSchema"].(map[string]any)
				if inputSchema == nil {
					inputSchema = map[string]any{"type": "object", "properties": map[string]any{}}
				}

				schemaBytes, _ := json.Marshal(inputSchema)

				w := &toolWrapper{
					client:      c,
					name:        "mcp_" + name + "_" + toolName,
					origName:    toolName,
					description: desc,
					parameters:  json.RawMessage(schemaBytes),
				}

				ts.Add(w)

				slog.Debug("MCP tool registered", "server", name, "tool", w.name)
			}
			slog.Info("MCP server connected", "server", name, "tools", len(toolDefs))
			m.clients = append(m.clients, c)
		}
	})
}

// Close stops all subprocess-based MCP servers owned by this manager.
func (m *Manager) Close() {
	for _, c := range m.clients {
		if c.cmd != nil && c.cmd.Process != nil {
			c.cmd.Process.Kill() //nolint:errcheck
		}
	}
}

// toServerConfig converts a config-layer MCPServerConfig to the internal ServerConfig.
func toServerConfig(c toolcfg.MCPServerConfig) ServerConfig {
	return ServerConfig{
		Command: c.Command,
		Args:    c.Args,
		Env:     c.Env,
		URL:     c.URL,
		Headers: c.Headers,
	}
}
