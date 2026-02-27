package schema

import "context"

type AgentSettings struct {
	Model        string
	MaxIter      int
	Temperature  float64
	MaxTokens    int
	MemoryWindow int
}

func NewAgentSettings(model string, maxIter int, temperature float64, maxTokens int, memoryWindow int) AgentSettings {
	return AgentSettings{
		Model:        model,
		MaxIter:      maxIter,
		Temperature:  temperature,
		MaxTokens:    maxTokens,
		MemoryWindow: memoryWindow,
	}
}

type AgentLooper interface {
	// ProcessDirect is for processing messages that bypass the normal bus flow,
	// e.g. system messages or subagent messages.
	ProcessDirect(ctx context.Context, content, key, channel, chatId string) string
	// Run starts the main agent loop,
	// processing messages from the bus until context is cancelled.
	Run(ctx context.Context) error
}
