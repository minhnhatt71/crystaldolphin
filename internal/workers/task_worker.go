package workers

import (
	"context"
	"fmt"

	contractagent "github.com/crystaldolphin/crystaldolphin/internal/modeling/agent"
	"github.com/crystaldolphin/crystaldolphin/internal/modeling/llmprovider"
	"github.com/crystaldolphin/crystaldolphin/internal/modeling/prompt"
)

// TaskWorker handles a background task delegated by the main agent (subagent turns).
// It uses a minimal system prompt — no memory, skills, or session history —
// and operates with a restricted tool registry (no message, spawn, or cron tools).
type TaskWorker struct {
	agent     contractagent.Agent
	settings  llmprovider.Settings
	workspace string
}

// NewTaskWorker constructs a TaskWorker.
func NewTaskWorker(agent contractagent.Agent, settings llmprovider.Settings, workspace string) *TaskWorker {
	return &TaskWorker{agent: agent, settings: settings, workspace: workspace}
}

// Process implements Worker.
func (w *TaskWorker) Process(ctx context.Context, req WorkerRequest) (WorkerResult, error) {
	systemPrompt := fmt.Sprintf(
		"You are a task agent handling a background task delegated by the main agent.\n\n"+
			"Rules:\n"+
			"- Stay focused on the assigned task; do not pursue side tasks.\n"+
			"- Do not send messages to users directly.\n"+
			"- Use only the tools available to you.\n\n"+
			"Workspace: %s",
		w.workspace,
	)
	userContent := req.Content

	prompts := prompt.NewList(
		prompt.New("system", &systemPrompt),
		prompt.NewUser(&userContent),
	)

	result, err := w.agent.Chat(ctx, prompts)
	if err != nil {
		return WorkerResult{}, err
	}

	return WorkerResult{Content: result}, nil
}
