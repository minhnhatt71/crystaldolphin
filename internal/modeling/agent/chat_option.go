package agent

import "github.com/crystaldolphin/crystaldolphin/internal/modeling/prompt"

// ChatArgs holds the resolved options for an Agent.Chat call.
type ChatArgs struct {
	Prompt prompt.Prompts
}

// ChatOption is a functional option applied to ChatArgs.
type ChatOption func(*ChatArgs)

// WithPrompt sets the initial Prompts for the chat call.
func WithPrompt(prompts prompt.Prompts) ChatOption {
	return func(c *ChatArgs) {
		c.Prompt = prompts
	}
}

// RetrieveChatArgs applies opts and returns the resolved ChatArgs.
func RetrieveChatArgs(opts ...ChatOption) *ChatArgs {
	args := &ChatArgs{}
	for _, opt := range opts {
		opt(args)
	}
	return args
}
