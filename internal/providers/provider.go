// Package providers defines the LLM provider interface and shared response types.
// Concrete implementations are in openai.go and codex.go (Phase 3).
package providers

import "github.com/crystaldolphin/crystaldolphin/internal/schema"

// ChatOptions configures a single LLM chat request.
// The canonical definition lives in internal/interfaces; this alias keeps
// existing code compiling without changes.
type ChatOptions = schema.ChatOptions

// ToolCallRequest represents one tool invocation requested by the LLM.
// The canonical definition lives in internal/interfaces; this alias keeps
// existing code compiling without changes.
type ToolCallRequest = schema.ToolCallRequest

// LLMResponse is the normalised response from any LLM provider.
// The canonical definition lives in internal/interfaces; this alias keeps
// existing code compiling without changes.
type LLMResponse = schema.LLMResponse

// LLMProvider is the interface every LLM backend must satisfy.
// The canonical definition lives in internal/interfaces; this alias keeps
// existing code compiling without changes.
type LLMProvider = schema.LLMProvider

// MessageHistory is the ordered list of messages exchanged with the LLM.
// The canonical definition lives in internal/interfaces; this alias keeps
// existing code compiling without changes.
type MessageHistory = schema.Messages

// NewMessageHistory returns an empty MessageHistory ready for use.
// The canonical constructor lives in internal/interfaces; this alias keeps
// existing code compiling without changes.
var NewMessageHistory = schema.NewMessages

// ToolCallDict is a typed representation of a tool-call request.
// The canonical definition lives in internal/interfaces; this alias keeps
// existing code compiling without changes.
type ToolCallDict = schema.ToolCallDict

// Message is one typed entry in the conversation history.
// The canonical definition lives in internal/interfaces; this alias keeps
// existing code compiling without changes.
type Message = schema.Message

// ContentBlock is a single block in a multimodal user message.
// The canonical definition lives in internal/interfaces; this alias keeps
// existing code compiling without changes.
type ContentBlock = schema.ContentBlock

// ToolCall represents one function call in an assistant message.
// The canonical definition lives in internal/interfaces; this alias keeps
// existing code compiling without changes.
type ToolCall = schema.ToolCall
