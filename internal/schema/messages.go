package schema

import "encoding/json"

// Messages is the ordered list of messages exchanged with the LLM.
// It owns typed append methods so callers never construct raw maps.
type Messages struct {
	Messages []Message
}

// NewMessages returns a Messages initialised with the given messages.
// Called with no arguments it returns an empty Messages ready for use.
func NewMessages(msgs ...Message) Messages {
	if len(msgs) == 0 {
		return Messages{Messages: make([]Message, 0)}
	}
	out := make([]Message, len(msgs))
	copy(out, msgs)
	return Messages{Messages: out}
}

// AddSystem appends a system message.
func (mh *Messages) AddSystem(content string) {
	mh.Messages = append(mh.Messages, Message{
		Role:    "system",
		Content: content,
	})
}

// AddUser appends a user message. content may be a plain string or
// []ContentBlock for multimodal messages.
func (mh *Messages) AddUser(content any) {
	mh.Messages = append(mh.Messages, Message{
		Role:    "user",
		Content: content,
	})
}

// AddAssistant appends an assistant message with optional tool calls and
// reasoning content.
func (mh *Messages) AddAssistant(content *string, toolCalls []ToolCall, reasoningContent *string) {
	mh.Messages = append(mh.Messages, Message{
		Role:             "assistant",
		Content:          content,
		ToolCalls:        toolCalls,
		ReasoningContent: reasoningContent,
	})
}

// AddToolResult appends a tool-result message.
func (mh *Messages) AddToolResult(toolCallID, toolName, result string) {
	mh.Messages = append(mh.Messages, Message{
		Role:       "tool",
		Content:    result,
		ToolCallID: toolCallID,
		ToolName:   toolName,
	})
}

func (mh *Messages) HashKey() ([]byte, error) {
	return json.Marshal(mh.Messages)
}

// Append copies all messages from other into mh.
func (mh *Messages) Append(other Messages) {
	mh.Messages = append(mh.Messages, other.Messages...)
}

// Clone returns a deep copy of mh with an independent backing slice.
func (mh *Messages) Clone() Messages {
	cloned := make([]Message, len(mh.Messages))
	copy(cloned, mh.Messages)
	return Messages{Messages: cloned}
}
