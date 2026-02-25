package interfaces

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

// ToolCallDict is an alias for ToolCall kept for backward compatibility.
type ToolCallDict = ToolCall

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
	ToolCallID       string  // "tool" role only
	ToolName         string  // "tool" role only
	ReasoningContent *string // "assistant" role only
}

// MessageHistory is the ordered list of messages exchanged with the LLM.
// It owns typed append methods so callers never construct raw maps.
type MessageHistory struct {
	Messages []Message
}

// NewMessageHistory returns an empty MessageHistory ready for use.
func NewMessageHistory() MessageHistory {
	return MessageHistory{Messages: make([]Message, 0)}
}

// AddSystem appends a system message.
func (mh *MessageHistory) AddSystem(content string) {
	mh.Messages = append(mh.Messages, Message{
		Role:    "system",
		Content: content,
	})
}

// AddUser appends a user message. content may be a plain string or
// []ContentBlock for multimodal messages.
func (mh *MessageHistory) AddUser(content any) {
	mh.Messages = append(mh.Messages, Message{
		Role:    "user",
		Content: content,
	})
}

// AddAssistant appends an assistant message with optional tool calls and
// reasoning content.
func (mh *MessageHistory) AddAssistant(content *string, toolCalls []ToolCall, reasoningContent *string) {
	mh.Messages = append(mh.Messages, Message{
		Role:             "assistant",
		Content:          content,
		ToolCalls:        toolCalls,
		ReasoningContent: reasoningContent,
	})
}

// AddToolResult appends a tool-result message.
func (mh *MessageHistory) AddToolResult(toolCallID, toolName, result string) {
	mh.Messages = append(mh.Messages, Message{
		Role:       "tool",
		Content:    result,
		ToolCallID: toolCallID,
		ToolName:   toolName,
	})
}

// Append copies all messages from other into mh.
func (mh *MessageHistory) Append(other MessageHistory) {
	mh.Messages = append(mh.Messages, other.Messages...)
}
