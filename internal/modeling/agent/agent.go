package agent

import (
	"context"

	"github.com/crystaldolphin/crystaldolphin/internal/modeling/prompt"
)

// Agent is the domain interface for an LLM-backed agent that can hold a
// multi-turn conversation. Concrete implementations live outside modeling/.
type Agent interface {
	Chat(ctx context.Context, prompts prompt.Prompts, opts ...ChatOption) (string, error)
}
