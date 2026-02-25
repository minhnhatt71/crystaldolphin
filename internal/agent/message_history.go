package agent

import "github.com/crystaldolphin/crystaldolphin/internal/interfaces"

// MessageHistory is the ordered list of messages exchanged with the LLM.
// The canonical definition lives in internal/interfaces; this alias keeps
// existing code in the agent package compiling without changes.
type MessageHistory = interfaces.MessageHistory

// NewMessageHistory returns an empty MessageHistory ready for use.
// The canonical constructor lives in internal/interfaces; this alias keeps
// existing code in the agent package compiling without changes.
var NewMessageHistory = interfaces.NewMessageHistory

// ToolCallDict is a typed representation of a tool-call request.
// The canonical definition lives in internal/interfaces; this alias keeps
// existing code in the agent package compiling without changes.
type ToolCallDict = interfaces.ToolCallDict

// ToolCall represents one function call in an assistant message.
// The canonical definition lives in internal/interfaces; this alias keeps
// existing code in the agent package compiling without changes.
type ToolCall = interfaces.ToolCall

// Message is one typed entry in the conversation history.
// The canonical definition lives in internal/interfaces; this alias keeps
// existing code in the agent package compiling without changes.
type Message = interfaces.Message
