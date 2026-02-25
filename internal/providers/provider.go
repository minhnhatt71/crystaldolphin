// Package providers defines the LLM provider interface and shared response types.
// Concrete implementations are in openai.go and codex.go (Phase 3).
package providers

import "github.com/crystaldolphin/crystaldolphin/internal/interfaces"

// ChatOptions configures a single LLM chat request.
// The canonical definition lives in internal/interfaces; this alias keeps
// existing code compiling without changes.
type ChatOptions = interfaces.ChatOptions

// ToolCallRequest represents one tool invocation requested by the LLM.
// The canonical definition lives in internal/interfaces; this alias keeps
// existing code compiling without changes.
type ToolCallRequest = interfaces.ToolCallRequest

// LLMResponse is the normalised response from any LLM provider.
// The canonical definition lives in internal/interfaces; this alias keeps
// existing code compiling without changes.
type LLMResponse = interfaces.LLMResponse

// LLMProvider is the interface every LLM backend must satisfy.
// The canonical definition lives in internal/interfaces; this alias keeps
// existing code compiling without changes.
type LLMProvider = interfaces.LLMProvider

// MessageHistory is the ordered list of messages exchanged with the LLM.
// The canonical definition lives in internal/interfaces; this alias keeps
// existing code compiling without changes.
type MessageHistory = interfaces.MessageHistory

// NewMessageHistory returns an empty MessageHistory ready for use.
// The canonical constructor lives in internal/interfaces; this alias keeps
// existing code compiling without changes.
var NewMessageHistory = interfaces.NewMessageHistory

// ToolCallDict is a typed representation of a tool-call request.
// The canonical definition lives in internal/interfaces; this alias keeps
// existing code compiling without changes.
type ToolCallDict = interfaces.ToolCallDict

// Message is one typed entry in the conversation history.
// The canonical definition lives in internal/interfaces; this alias keeps
// existing code compiling without changes.
type Message = interfaces.Message

// ContentBlock is a single block in a multimodal user message.
// The canonical definition lives in internal/interfaces; this alias keeps
// existing code compiling without changes.
type ContentBlock = interfaces.ContentBlock

// ToolCall represents one function call in an assistant message.
// The canonical definition lives in internal/interfaces; this alias keeps
// existing code compiling without changes.
type ToolCall = interfaces.ToolCall
