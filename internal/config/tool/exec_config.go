package tool

// ExecToolConfig configures the shell-exec tool.
type ExecToolConfig struct {
	Timeout int `json:"timeout"` // seconds
}

func DefaultExecToolConfig() ExecToolConfig {
	return ExecToolConfig{Timeout: 60}
}
