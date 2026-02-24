package interfaces

import "context"

// Spawner is the interface the spawn tool uses to create background subagents.
// Implemented by agent.SubagentManager. Defined here to avoid an import cycle.
type Spawner interface {
	Spawn(ctx context.Context, task, label, originChannel, originChatID string) (string, error)
}
