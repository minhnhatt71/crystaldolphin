package tool

// ToolsConfig groups all tool-level settings.
type ToolsConfig struct {
	Web                 WebToolsConfig             `json:"web"`
	Exec                ExecToolConfig             `json:"exec"`
	RestrictToWorkspace bool                       `json:"restrictToWorkspace"`
	MCPServers          map[string]MCPServerConfig `json:"mcpServers"`
}

func DefaultToolConfigs() ToolsConfig {
	return ToolsConfig{
		Web:        DefaultWebToolsConfig(),
		Exec:       DefaultExecToolConfig(),
		MCPServers: map[string]MCPServerConfig{},
	}
}
