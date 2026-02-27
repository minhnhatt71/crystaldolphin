package mcp

// ServerConfig holds the connection parameters for a single MCP server.
type ServerConfig struct {
	Command string
	Args    []string
	Env     map[string]string
	URL     string
	Headers map[string]string
}
