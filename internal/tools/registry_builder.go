package tools

import "github.com/crystaldolphin/crystaldolphin/internal/schema"

// RegistryBuilder accumulates tools during the construction phase.
// Call Build() to produce an immutable Registry ready for use.
type RegistryBuilder struct {
	tools map[string]schema.Tool
}

// NewRegistryBuilder returns a fresh RegistryBuilder.
func NewRegistryBuilder() *RegistryBuilder {
	return &RegistryBuilder{tools: make(map[string]schema.Tool)}
}

// Tool adds a tool and returns the builder, enabling chaining.
func (b *RegistryBuilder) Tool(tool schema.Tool) *RegistryBuilder {
	b.tools[tool.Name()] = tool

	return b
}

// Build produces an immutable Registry from the accumulated tools.
func (b *RegistryBuilder) Build() *Registry {
	tools := make(map[string]schema.Tool, len(b.tools))
	for k, v := range b.tools {
		tools[k] = v
	}
	return &Registry{tools: tools}
}
