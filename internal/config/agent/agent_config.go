package agent

type AgentDefaults struct {
	Workspace    string  `json:"workspace"`
	Model        string  `json:"model"`
	MaxTokens    int     `json:"maxTokens"`
	Temperature  float64 `json:"temperature"`
	MaxToolIter  int     `json:"maxToolIterations"`
	MemoryWindow int     `json:"memoryWindow"`
}

type AgentsConfig struct {
	Defaults AgentDefaults `json:"defaults"`
}

func defaultAgentDefaults() AgentDefaults {
	return AgentDefaults{
		Workspace:    "~/.nanobot/workspace",
		Model:        "gemini/gemini-2.5-pro",
		MaxTokens:    8192,
		Temperature:  0.7,
		MaxToolIter:  20,
		MemoryWindow: 50,
	}
}

func DefaultAgentsConfig() AgentsConfig {
	return AgentsConfig{Defaults: defaultAgentDefaults()}
}
