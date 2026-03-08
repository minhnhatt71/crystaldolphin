package agent

import "github.com/crystaldolphin/crystaldolphin/internal/modeling/prompt"

type chatArgs struct {
	prompt prompt.Prompts
}

type ChatOption func(*chatArgs)

func WithPrompt(prompts prompt.Prompts) ChatOption {
	return func(c *chatArgs) {
		c.prompt = prompts
	}
}

func retrieveChatArgs(opts ...ChatOption) *chatArgs {
	args := &chatArgs{}

	for _, opt := range opts {
		opt(args)
	}

	return args
}
