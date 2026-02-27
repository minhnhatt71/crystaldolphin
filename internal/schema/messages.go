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

// Add appends a raw message. Callers should prefer the typed AddX methods
func (mh *Messages) Add(msg Message) {
	mh.Messages = append(mh.Messages, msg)
}

// AddSystem appends a system message.
func (mh *Messages) AddSystem(content string) {
	mh.Messages = append(mh.Messages, Message{
		Role:    RoleSystem,
		Content: content,
	})
}

// AddUser appends a user message. content may be a plain string or
// []ContentBlock for multimodal messages.
func (mh *Messages) AddUser(content any) {
	mh.Messages = append(mh.Messages, Message{
		Role:    RoleUser,
		Content: content,
	})
}

// AddAssistant appends an assistant message with optional tool calls and
// reasoning content.
func (mh *Messages) AddAssistant(content *string, toolCalls []ToolCall, reasoningContent *string) {
	mh.Messages = append(mh.Messages, Message{
		Role:             RoleAssistant,
		Content:          content,
		ToolCalls:        toolCalls,
		ReasoningContent: reasoningContent,
	})
}

// AddToolResult appends a tool-result message.
func (mh *Messages) AddToolResult(toolCallID, toolName, result string) {
	mh.Messages = append(mh.Messages, Message{
		Role:       RoleTool,
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

// Len returns the number of messages in mh, treating nil as empty.
func (mh Messages) Len() int {
	if mh.Messages == nil {
		return 0
	}

	return len(mh.Messages)
}

// Copy returns a deep copy of mh with an independent backing slice.
func (mh *Messages) Copy() Messages {
	cloned := make([]Message, len(mh.Messages))
	copy(cloned, mh.Messages)
	return Messages{Messages: cloned}
}
