package workers

import (
	"context"
	"fmt"
	"strings"

	contractagent "github.com/crystaldolphin/crystaldolphin/internal/modeling/agent"
	"github.com/crystaldolphin/crystaldolphin/internal/modeling/llmprovider"
	"github.com/crystaldolphin/crystaldolphin/internal/modeling/prompt"
	"github.com/crystaldolphin/crystaldolphin/internal/modeling/session"
	"github.com/crystaldolphin/crystaldolphin/internal/modeling/skills"
)

// WorkerRequest carries everything a worker needs to build prompts and run the agent.
// Infrastructure data (history, long-term memory) is pre-resolved by the caller —
// workers perform no store I/O of their own.
type WorkerRequest struct {
	Channel        string
	ChatID         string
	SenderID       string
	Content        string
	Media          []string
	Metadata       map[string]any
	History        []session.SessionEntry
	LongTermMemory string // pre-read by the service layer; empty if none
}

// WorkerResult is returned from Worker.Process.
type WorkerResult struct {
	Content   string
	ToolsUsed []string
}

// Worker processes a single agent turn given a fully resolved WorkerRequest.
type Worker interface {
	Process(ctx context.Context, req WorkerRequest) (WorkerResult, error)
}

// ChatWorker handles a user-facing conversation turn. It builds the full system
// prompt (memory, skills, channel context), converts the session history into
// prompts, and delegates the LLM↔tool loop to the underlying Agent.
type ChatWorker struct {
	agent        contractagent.Agent
	settings     llmprovider.Settings
	skillsLoader skills.Loader
}

// NewChatWorker constructs a ChatWorker.
func NewChatWorker(agent contractagent.Agent, settings llmprovider.Settings, skillsLoader skills.Loader) *ChatWorker {
	return &ChatWorker{agent: agent, settings: settings, skillsLoader: skillsLoader}
}

// Process implements Worker.
func (w *ChatWorker) Process(ctx context.Context, req WorkerRequest) (WorkerResult, error) {
	systemContent := w.buildSystemPrompt(req)
	prompts := prompt.NewList(prompt.New("system", &systemContent))

	for _, entry := range req.History {
		content := entry.Content()
		switch entry.Role() {
		case session.RoleUser:
			prompts = prompts.Add(prompt.NewUser(&content))
		case session.RoleAssistant:
			prompts = prompts.Add(prompt.NewAssistant(&content, nil, nil))
		}
	}

	userContent := req.Content
	prompts = prompts.Add(prompt.NewUser(&userContent))

	result, err := w.agent.Chat(ctx, prompts)
	if err != nil {
		return WorkerResult{}, err
	}

	return WorkerResult{Content: result}, nil
}

func (w *ChatWorker) buildSystemPrompt(req WorkerRequest) string {
	var parts []string

	if req.LongTermMemory != "" {
		parts = append(parts, "# Memory\n\n"+req.LongTermMemory)
	}

	if w.skillsLoader != nil {
		if alwaysSkills := w.skillsLoader.GetAlwaysSkills(); len(alwaysSkills) > 0 {
			if content := w.skillsLoader.LoadSkillsForContext(alwaysSkills); content != "" {
				parts = append(parts, "# Active Skills\n\n"+content)
			}
		}
		if summary := w.skillsLoader.BuildSkillsSummary(); summary != "" {
			parts = append(parts, "# Skills\n\n"+summary)
		}
	}

	if req.Channel != "" && req.ChatID != "" {
		parts = append(parts, fmt.Sprintf("## Current Session\nChannel: %s\nChat ID: %s", req.Channel, req.ChatID))
	}

	return strings.Join(parts, "\n\n---\n\n")
}
