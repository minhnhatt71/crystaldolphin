package schema

import (
	"context"

	"github.com/crystaldolphin/crystaldolphin/internal/bus"
)

type Task struct {
	id          string
	label       string
	description string
}

func NewTask(id, label, description string) Task {
	return Task{
		id:          id,
		label:       label,
		description: description,
	}
}

func (t Task) Id() string          { return t.id }
func (t Task) Label() string       { return t.label }
func (t Task) Description() string { return t.description }

// Spawner is the interface the spawn tool uses to create background subagents.
// Implemented by agent.SubagentManager. Defined here to avoid an import cycle.
type Spawner interface {
	Spawn(ctx context.Context, task, label string, originChannel bus.Channel, originChatID string) (string, error)
}
