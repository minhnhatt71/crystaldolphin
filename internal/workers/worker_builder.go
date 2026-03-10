package workers

import (
	contractagent "github.com/crystaldolphin/crystaldolphin/internal/modeling/agent"
	"github.com/crystaldolphin/crystaldolphin/internal/modeling/llmprovider"
	"github.com/crystaldolphin/crystaldolphin/internal/modeling/skills"
)

// ChatWorkerBuilder constructs a ChatWorker with a fluent API.
type ChatWorkerBuilder struct {
	agent        contractagent.Agent
	settings     llmprovider.Settings
	skillsLoader skills.Loader
}

// NewChatWorkerBuilder returns a blank ChatWorkerBuilder.
func NewChatWorkerBuilder() *ChatWorkerBuilder {
	return &ChatWorkerBuilder{}
}

func (b *ChatWorkerBuilder) WithAgent(a contractagent.Agent) *ChatWorkerBuilder {
	b.agent = a
	return b
}

func (b *ChatWorkerBuilder) WithSettings(s llmprovider.Settings) *ChatWorkerBuilder {
	b.settings = s
	return b
}

func (b *ChatWorkerBuilder) WithSkillsLoader(l skills.Loader) *ChatWorkerBuilder {
	b.skillsLoader = l
	return b
}

// Build constructs a ChatWorker. Returns nil if agent is not set.
func (b *ChatWorkerBuilder) Build() *ChatWorker {
	if b.agent == nil {
		return nil
	}
	return NewChatWorker(b.agent, b.settings, b.skillsLoader)
}
