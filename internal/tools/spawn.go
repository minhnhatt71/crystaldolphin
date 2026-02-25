package tools

import (
	"context"
	"encoding/json"

	"github.com/crystaldolphin/crystaldolphin/internal/schema"
)

// SpawnTool spawns a background subagent to handle a task asynchronously.
// The origin channel/chatID for result delivery is read from TurnContext.
type SpawnTool struct {
	spawner schema.Spawner
}

// NewSpawnTool creates a SpawnTool backed by the given Spawner.
func NewSpawnTool(spawner schema.Spawner) *SpawnTool {
	return &SpawnTool{spawner: spawner}
}

// Name of the tool
func (t *SpawnTool) Name() string { return "spawn" }

func (t *SpawnTool) Description() string {
	return "Spawn a subagent to handle a task in the background. " +
		"Use this for complex or time-consuming tasks that can run independently. " +
		"The subagent will complete the task and report back when done."
}

// Parameters returns the JSON Schema for the tool's parameters.
func (t *SpawnTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"task": {
				"type": "string",
				"description": "The task for the subagent to complete"
			},
			"label": {
				"type": "string",
				"description": "Optional short label for the task (for display)"
			}
		},
		"required": ["task"]
	}`)
}

// Execute spawns a subagent with the given task and label, and returns immediately.
func (t *SpawnTool) Execute(ctx context.Context, params map[string]any) (string, error) {
	task, _ := params["task"].(string)
	if task == "" {
		return "Error: task is required", nil
	}
	label, _ := params["label"].(string)

	tc := TurnCtx(ctx)
	originChannel := tc.Channel
	if originChannel == "" {
		originChannel = "cli"
	}
	originChatID := tc.ChatID
	if originChatID == "" {
		originChatID = "direct"
	}

	result, err := t.spawner.Spawn(ctx, task, label, originChannel, originChatID)
	if err != nil {
		return "Error spawning subagent: " + err.Error(), nil
	}
	return result, nil
}
