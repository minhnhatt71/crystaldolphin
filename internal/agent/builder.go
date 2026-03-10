package agent

import (
	contract "github.com/crystaldolphin/crystaldolphin/internal/modeling/agent"
	"github.com/crystaldolphin/crystaldolphin/internal/modeling/llmprovider"
)

// Compile-time check that AgentBuilder satisfies the domain Builder interface.
var _ contract.Builder = (*AgentBuilder)(nil)

// AgentBuilder constructs an Agent with a fluent API.
type AgentBuilder struct {
	settings llmprovider.Settings
	provider llmprovider.LLMProvider
	registry contract.ToolHandlerRegistry
}

// NewBuilder returns a blank AgentBuilder as the domain Builder interface.
func NewBuilder() contract.Builder {
	return &AgentBuilder{}
}

// WithSettings sets the LLM settings (model, iterations, temperature, …).
func (b *AgentBuilder) WithSettings(s llmprovider.Settings) contract.Builder {
	b.settings = s
	return b
}

// WithProvider sets the LLM provider used to make chat requests.
func (b *AgentBuilder) WithProvider(p llmprovider.LLMProvider) contract.Builder {
	b.provider = p
	return b
}

// WithRegistry sets the tool handler registry.
func (b *AgentBuilder) WithRegistry(r contract.ToolHandlerRegistry) contract.Builder {
	b.registry = r
	return b
}

// Build constructs the Agent. Returns nil if any required field is missing.
func (b *AgentBuilder) Build() contract.Agent {
	if b.provider == nil || b.registry == nil {
		return nil
	}
	return New(b.settings, b.provider, b.registry)
}
