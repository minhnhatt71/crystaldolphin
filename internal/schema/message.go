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

func NewToolCall(id, name string, arguments map[string]any) ToolCall {
	return ToolCall{
		ID:        id,
		Name:      name,
		Arguments: arguments,
	}
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

type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool"
)

// Message is one entry in the conversation history.
//
// Role is one of: RoleSystem, RoleUser, RoleAssistant, RoleTool.
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
	Role             MessageRole
	Content          any // string | *string | []ContentBlock
	ToolCalls        []ToolCall
	ToolCallID       string   // "tool" role only
	ToolName         string   // "tool" role only
	ReasoningContent *string  // "assistant" role only
	ToolsUsed        []string // session-only: names of tools used this turn; not sent to LLM
}

func NewSystemMessage(content any) Message {
	return Message{
		Role:    RoleSystem,
		Content: content,
	}
}

func NewUserMessage(content any) Message {
	return Message{
		Role:    RoleUser,
		Content: content,
	}
}

func NewAssistantMessage(content *string, toolCalls []ToolCall, reasoningContent *string) Message {
	return Message{
		Role:             RoleAssistant,
		Content:          content,
		ToolCalls:        toolCalls,
		ReasoningContent: reasoningContent,
	}
}

func NewToolResultMessage(toolCallID, toolName, result string) Message {
	return Message{
		Role:       RoleTool,
		Content:    result,
		ToolCallID: toolCallID,
		ToolName:   toolName,
	}
}
