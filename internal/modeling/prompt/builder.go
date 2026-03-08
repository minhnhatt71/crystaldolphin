package prompt

type createArgs struct {
	tools     []Tool
	reasoning *string
}

type Options func(*createArgs)

func New(role string, content *string, opts ...Options) Prompt {
	args := &createArgs{}

	for _, opt := range opts {
		opt(args)
	}

	return Prompt{
		role:      role,
		tools:     args.tools,
		content:   content,
		reasoning: args.reasoning,
	}
}

func NewAssistant(content *string, used Tools, reasoning *string) Prompt {
	return Prompt{
		role:      "assistant",
		content:   content,
		tools:     used.List(),
		reasoning: reasoning,
	}
}

func NewUser(content *string) Prompt {
	return Prompt{
		role:    "user",
		content: content,
	}
}

func NewToolCall(tool Tool, result *string) Prompt {
	return Prompt{
		role:    "tool",
		tools:   []Tool{tool},
		content: result,
	}
}

// NewToolCalls creates a tool-result prompt with the given tools and result.
func NewToolCalls(used []Tool, result *string) Prompt {
	return Prompt{
		role:    "tool",
		tools:   used,
		content: result,
	}
}

func WithTool(tool Tool) Options {
	return func(p *createArgs) {
		p.tools = append(p.tools, tool)
	}
}

func WithTools(tools []Tool) Options {
	return func(p *createArgs) {
		p.tools = tools
	}
}

func WithReasoning(content *string) Options {
	return func(p *createArgs) {
		p.reasoning = content
	}
}
