package schema

import "encoding/json"

// ContentBlock is a single block in a multimodal user message
// (e.g. an image_url block alongside a text block).
type ContentBlock struct {
	Type     string         // "text" | "image_url"
	Text     string         // when Type == "text"
	ImageURL map[string]any // when Type == "image_url"
}

// ToolCall represents one function call in an assistant message.
type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]any
}

// ToWireMap serialises a ToolCall into the OpenAI wire-format map.
// Used by provider implementations when building the JSON request body.
func (tc ToolCall) ToWireMap() map[string]any {
	argsJSON, _ := json.Marshal(tc.Arguments)
	return map[string]any{
		"id":   tc.ID,
		"type": "function",
		"function": map[string]any{
			"name":      tc.Name,
			"arguments": string(argsJSON),
		},
	}
}

// Message is one entry in the conversation history.
//
// Role is one of: "system", "user", "assistant", "tool".
//
// Content holds the message text or content blocks:
//   - system / tool: plain string
//   - user: string or []ContentBlock (multimodal)
//   - assistant: *string (may be nil when only tool calls are present)
//
// ToolCalls is populated for assistant messages that invoke tools.
// ToolCallID and ToolName are set for tool-result messages.
// ReasoningContent carries the thinking block from models like DeepSeek-R1.
type Message struct {
	Role             string
	Content          any // string | *string | []ContentBlock
	ToolCalls        []ToolCall
	ToolCallID       string   // "tool" role only
	ToolName         string   // "tool" role only
	ReasoningContent *string  // "assistant" role only
	ToolsUsed        []string // session-only: names of tools used this turn; not sent to LLM
}

func NewSystemMessage(content any) Message {
	return Message{
		Role:    "system",
		Content: content,
	}
}

func NewUserMessage(content any) Message {
	return Message{
		Role:    "user",
		Content: content,
	}
}

func NewAssistantMessage(content *string, toolCalls []ToolCall, reasoningContent *string) Message {
	return Message{
		Role:             "assistant",
		Content:          content,
		ToolCalls:        toolCalls,
		ReasoningContent: reasoningContent,
	}
}

func NewToolResultMessage(toolCallID, toolName, result string) Message {
	return Message{
		Role:       "tool",
		Content:    result,
		ToolCallID: toolCallID,
		ToolName:   toolName,
	}
}
