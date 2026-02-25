package agent

import "github.com/crystaldolphin/crystaldolphin/internal/interfaces"

// Messages is the ordered list of messages exchanged with the LLM.
// The canonical definition lives in internal/interfaces; this alias keeps
// existing code in the agent package compiling without changes.
type Messages = interfaces.Messages

// NewMessages returns an empty Messages ready for use.
// The canonical constructor lives in internal/interfaces; this alias keeps
// existing code in the agent package compiling without changes.
var NewMessages = interfaces.NewMessages

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
