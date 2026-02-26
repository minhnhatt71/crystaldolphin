package schema

import "context"

// ChatOptions configures a single LLM chat request.
type ChatOptions struct {
	Model       string
	MaxTokens   int
	Temperature float64
}

type ToolCallRequest struct {
	Id        string
	Name      string
	Arguments map[string]any
}

type ToolCallResponse = ToolCallRequest

// LLMResponse is the normalised response from any LLM provider.
type LLMResponse struct {
	Content          *string // nil when the response contains only tool calls
	ToolCalls        []ToolCallResponse
	FinishReason     string
	Usage            map[string]int // "input_tokens", "output_tokens"
	ReasoningContent *string        // DeepSeek-R1 / Kimi thinking block
}

// HasToolCalls reports whether the response contains at least one tool call.
func (r LLMResponse) HasToolCalls() bool { return len(r.ToolCalls) > 0 }

// LLMProvider is the interface every LLM backend must satisfy.
type LLMProvider interface {
	Chat(ctx context.Context, messages Messages, tools []map[string]any, opts ChatOptions) (LLMResponse, error)
	DefaultModel() string
}

func NewChatOptions(model string, maxTokens int, temperature float64) ChatOptions {
	return ChatOptions{
		Model:       model,
		MaxTokens:   maxTokens,
		Temperature: temperature,
	}
}
