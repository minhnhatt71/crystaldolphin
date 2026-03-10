package agent

import "github.com/crystaldolphin/crystaldolphin/internal/modeling/llmprovider"

// Builder is the domain interface for constructing an Agent.
// Concrete implementations (e.g. internal/agent.AgentBuilder) live outside modeling/.
type Builder interface {
	WithSettings(s llmprovider.Settings) Builder
	WithProvider(p llmprovider.LLMProvider) Builder
	WithRegistry(r ToolHandlerRegistry) Builder
	Build() Agent
}
