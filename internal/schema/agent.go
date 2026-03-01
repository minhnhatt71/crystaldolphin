package schema

import (
	"context"

	"github.com/crystaldolphin/crystaldolphin/internal/bus"
)

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
	// ProcessDirect processes a message outside the bus (CLI, cron, heartbeat).
	// Returns the final text response.
	ProcessDirect(ctx context.Context, msg bus.AgentMessage) string
	// Run starts the main agent loop,
	// processing messages from the bus until context is cancelled.
	Run(ctx context.Context) error
}

// Agent executes a single LLM ↔ tool loop for one request.
type Agent interface {
	Execute(ctx context.Context, conversation Messages, onProgress func(string)) (string, []string)
}
