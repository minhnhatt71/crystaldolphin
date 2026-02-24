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
