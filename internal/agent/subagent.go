package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/crystaldolphin/crystaldolphin/internal/bus"
	"github.com/crystaldolphin/crystaldolphin/internal/providers"
	"github.com/crystaldolphin/crystaldolphin/internal/tools"
)

// SubagentManager manages background goroutine tasks (subagents).
// Each subagent has its own isolated tool registry (no message/spawn tools).
// Mirrors nanobot's Python SubagentManager.
type SubagentManager struct {
	provider            providers.LLMProvider
	workspace           string
	bus                 *bus.MessageBus
	model               string
	temperature         float64
	maxTokens           int
	braveAPIKey         string
	execTimeout         int
	restrictToWorkspace bool

	mu      sync.Mutex
	running map[string]context.CancelFunc
}

// NewSubagentManager creates a SubagentManager.
func NewSubagentManager(
	provider providers.LLMProvider,
	workspace string,
	msgBus *bus.MessageBus,
	model string,
	temperature float64,
	maxTokens int,
	braveAPIKey string,
	execTimeout int,
	restrictToWorkspace bool,
) *SubagentManager {
	return &SubagentManager{
		provider:            provider,
		workspace:           workspace,
		bus:                 msgBus,
		model:               model,
		temperature:         temperature,
		maxTokens:           maxTokens,
		braveAPIKey:         braveAPIKey,
		execTimeout:         execTimeout,
		restrictToWorkspace: restrictToWorkspace,
		running:             make(map[string]context.CancelFunc),
	}
}

// Spawn starts a background subagent goroutine and returns immediately.
// Implements tools.Spawner.
func (sm *SubagentManager) Spawn(
	ctx context.Context,
	task, label, originChannel, originChatID string,
) (string, error) {
	taskID := shortID()
	if label == "" {
		label = task
		if len(label) > 30 {
			label = label[:30] + "..."
		}
	}

	subCtx, cancel := context.WithCancel(context.Background()) // detached from caller
	sm.mu.Lock()
	sm.running[taskID] = cancel
	sm.mu.Unlock()

	go func() {
		defer func() {
			sm.mu.Lock()
			delete(sm.running, taskID)
			sm.mu.Unlock()
			cancel()
		}()
		sm.runSubagent(subCtx, taskID, task, label, originChannel, originChatID)
	}()

	slog.Info("Spawned subagent", "id", taskID, "label", label)
	return fmt.Sprintf("Subagent [%s] started (id: %s). I'll notify you when it completes.", label, taskID), nil
}

// RunningCount returns the number of currently running subagents.
func (sm *SubagentManager) RunningCount() int {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return len(sm.running)
}

func (sm *SubagentManager) runSubagent(
	ctx context.Context,
	taskID, task, label, originChannel, originChatID string,
) {
	slog.Info("Subagent starting", "id", taskID, "label", label)

	finalResult, err := sm.executeTask(ctx, task)
	if err != nil {
		finalResult = "Error: " + err.Error()
		slog.Error("Subagent failed", "id", taskID, "err", err)
	} else {
		slog.Info("Subagent completed", "id", taskID)
	}

	status := "completed successfully"
	if err != nil {
		status = "failed"
	}
	sm.announceResult(taskID, label, task, finalResult, status, originChannel, originChatID)
}

func (sm *SubagentManager) executeTask(ctx context.Context, task string) (string, error) {
	// Isolated tool registry — no message, no spawn tools.
	registry := tools.NewRegistry()
	allowedDir := ""
	if sm.restrictToWorkspace {
		allowedDir = sm.workspace
	}
	registry.Register(tools.NewReadFileTool(sm.workspace, allowedDir))
	registry.Register(tools.NewWriteFileTool(sm.workspace, allowedDir))
	registry.Register(tools.NewEditFileTool(sm.workspace, allowedDir))
	registry.Register(tools.NewListDirTool(sm.workspace, allowedDir))
	registry.Register(tools.NewExecTool(sm.workspace, sm.execTimeout, sm.restrictToWorkspace))
	registry.Register(tools.NewWebSearchTool(sm.braveAPIKey, 5))
	registry.Register(tools.NewWebFetchTool(0))

	systemPrompt := sm.buildPrompt(task)
	messages := []map[string]any{
		{"role": "system", "content": systemPrompt},
		{"role": "user", "content": task},
	}

	const maxIter = 15
	for i := 0; i < maxIter; i++ {
		resp, err := sm.provider.Chat(ctx, messages, registry.GetDefinitions(), providers.ChatOptions{
			Model:       sm.model,
			MaxTokens:   sm.maxTokens,
			Temperature: sm.temperature,
		})
		if err != nil {
			return "", err
		}

		if len(resp.ToolCalls) == 0 {
			content := ""
			if resp.Content != nil {
				content = *resp.Content
			}
			if content == "" {
				content = "Task completed but no final response was generated."
			}
			return content, nil
		}

		// Append assistant turn with tool calls.
		var tcDicts []map[string]any
		for _, tc := range resp.ToolCalls {
			argsJSON, _ := json.Marshal(tc.Arguments)
			tcDicts = append(tcDicts, map[string]any{
				"id":   tc.ID,
				"type": "function",
				"function": map[string]any{
					"name":      tc.Name,
					"arguments": string(argsJSON),
				},
			})
		}
		contentVal := any(nil)
		if resp.Content != nil {
			contentVal = *resp.Content
		}
		messages = append(messages, map[string]any{
			"role":       "assistant",
			"content":    contentVal,
			"tool_calls": tcDicts,
		})

		// Execute each tool.
		for _, tc := range resp.ToolCalls {
			slog.Debug("Subagent tool call", "id", taskID(ctx), "tool", tc.Name)
			result := registry.Execute(ctx, tc.Name, tc.Arguments)
			messages = append(messages, map[string]any{
				"role":         "tool",
				"tool_call_id": tc.ID,
				"name":         tc.Name,
				"content":      result,
			})
		}
	}
	return "Task completed (max iterations reached).", nil
}

// taskID is a helper that extracts the task ID stored in context (if any).
// Used only for logging; returns "" if not set.
func taskID(_ context.Context) string { return "" }

func (sm *SubagentManager) announceResult(
	taskID,
	label,
	task,
	result,
	status,
	originChannel,
	originChatID string,
) {
	content := fmt.Sprintf(`[Subagent '%s' %s]

Task: %s

Result:
%s

Summarize this naturally for the user. Keep it brief (1-2 sentences). Do not mention technical details like "subagent" or task IDs.`,
		label, status, task, result)

	sm.bus.Inbound <- bus.InboundMessage{
		Channel:   "system",
		SenderID:  "subagent",
		ChatID:    originChannel + ":" + originChatID,
		Content:   content,
		Timestamp: time.Now(),
	}
}

func (sm *SubagentManager) buildPrompt(task string) string {
	now := time.Now().Format("2006-01-02 15:04 (Monday)")
	tz, _ := time.Now().Zone()
	if tz == "" {
		tz = "UTC"
	}
	ws := expandHome(sm.workspace)
	goos := runtime.GOOS
	if goos == "darwin" {
		goos = "macOS"
	}

	_ = task // task is in the user message, not repeated in prompt
	return strings.Join([]string{
		"# Subagent",
		"",
		"## Current Time",
		now + " (" + tz + ")",
		"",
		"You are a subagent spawned by the main agent to complete a specific task.",
		"",
		"## Rules",
		"1. Stay focused - complete only the assigned task, nothing else",
		"2. Your final response will be reported back to the main agent",
		"3. Do not initiate conversations or take on side tasks",
		"4. Be concise but informative in your findings",
		"",
		"## What You Can Do",
		"- Read and write files in the workspace",
		"- Execute shell commands",
		"- Search the web and fetch web pages",
		"- Complete the task thoroughly",
		"",
		"## What You Cannot Do",
		"- Send messages directly to users (no message tool available)",
		"- Spawn other subagents",
		"- Access the main agent's conversation history",
		"",
		"## Workspace",
		"Your workspace is at: " + ws,
		"Skills are available at: " + ws + "/skills/ (read SKILL.md files as needed)",
		"OS: " + goos + " " + runtime.GOARCH,
		"",
		"When you have completed the task, provide a clear summary of your findings or actions.",
	}, "\n")
}

// shortID generates a short pseudo-unique ID (first 8 chars of a UUID-like value).
func shortID() string {
	return fmt.Sprintf("%08x", time.Now().UnixNano()&0xFFFFFFFF)
}
