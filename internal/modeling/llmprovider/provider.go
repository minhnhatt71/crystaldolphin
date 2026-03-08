package llmprovider

import (
	"context"

	"github.com/crystaldolphin/crystaldolphin/internal/modeling/prompt"
)

type LLMProvider interface {
	Chat(ctx context.Context, prompt prompt.Prompts, cfg Settings) (LLMResponse, error)
}
