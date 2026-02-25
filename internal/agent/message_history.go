package agent

import "github.com/crystaldolphin/crystaldolphin/internal/schema"

// Messages is the ordered list of messages exchanged with the LLM.
// The canonical definition lives in internal/interfaces; this alias keeps
// existing code in the agent package compiling without changes.
type Messages = schema.Messages

// NewMessages returns an empty Messages ready for use.
// The canonical constructor lives in internal/interfaces; this alias keeps
// existing code in the agent package compiling without changes.
var NewMessages = schema.NewMessages

// NewSystemMessage / NewUserMessage are aliases for the canonical constructors
// in internal/interfaces, kept here so agent-package code doesn't need to
// import interfaces directly.
var NewSystemMessage = schema.NewSystemMessage
var NewUserMessage = schema.NewUserMessage

// ToolCallDict is a typed representation of a tool-call request.
// The canonical definition lives in internal/interfaces; this alias keeps
// existing code in the agent package compiling without changes.
type ToolCallDict = schema.ToolCallDict

// ToolCall represents one function call in an assistant message.
// The canonical definition lives in internal/interfaces; this alias keeps
// existing code in the agent package compiling without changes.
type ToolCall = schema.ToolCall

// Message is one typed entry in the conversation history.
// The canonical definition lives in internal/interfaces; this alias keeps
// existing code in the agent package compiling without changes.
type Message = schema.Message
