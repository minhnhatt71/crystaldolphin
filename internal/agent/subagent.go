package agent

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/crystaldolphin/crystaldolphin/internal/bus"
	"github.com/crystaldolphin/crystaldolphin/internal/schema"
	"github.com/crystaldolphin/crystaldolphin/internal/shared/llmutils"
	"github.com/crystaldolphin/crystaldolphin/internal/tools"
)

// SubagentManager manages background goroutine tasks (subagents).
// Each subagent is constructed via AgentFactory.NewSubAgent() so it carries
// a restricted tool set (no message/spawn/cron tools).
type SubagentManager struct {
	factory *AgentFactory
	bus     *bus.AgentBus

	mu      sync.Mutex
	running map[string]context.CancelFunc
}

// NewSubagentManager creates a SubagentManager backed by the given factory.
func NewSubagentManager(factory *AgentFactory, bus *bus.AgentBus) *SubagentManager {
	return &SubagentManager{
		factory: factory,
		bus:     bus,
		running: make(map[string]context.CancelFunc),
	}
}

// Spawn starts a background subagent goroutine and returns immediately.
// Implements tools.Spawner.
func (sm *SubagentManager) Spawn(ctx context.Context, task, label string, originChannel bus.Channel, originChatID string) (string, error) {
	taskID := shortID()
	label = llmutils.StringOrDefault(label, task)
	label = llmutils.Truncate(label, 30)

	subctx, cancel := context.WithCancel(context.Background()) // detached from caller

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
		sm.runSubagent(subctx, taskID, task, label, originChannel, originChatID)
	}()

	slog.Info("Spawned subagent", "id", taskID, "label", label)
	return fmt.Sprintf("Subagent [%s] started (id: %s). I'll notify you when it completes.", label, taskID), nil
}

func (sm *SubagentManager) runSubagent(
	ctx context.Context,
	taskId, task, label string, originChannel bus.Channel, originChatId string,
) {
	slog.Info("Subagent starting", "id", taskId, "label", label)

	result, err := sm.executeTask(ctx, task, taskId)
	if err != nil {
		result = "Error: " + err.Error()
		slog.Error("Subagent failed", "id", taskId, "err", err)
	} else {
		slog.Info("Subagent completed", "id", taskId)
	}

	status := "completed successfully"
	if err != nil {
		status = "failed"
	}

	sm.announceResult(label, task, result, status, originChannel, originChatId)
}

func (sm *SubagentManager) executeTask(ctx context.Context, task, _ string) (string, error) {
	subAgent := sm.factory.NewSubAgent()

	conversation := schema.NewMessages(
		schema.NewSystemMessage(subAgent.buildSystemPrompt()),
		schema.NewUserMessage(task),
	)

	content, _ := subAgent.Execute(ctx, conversation, nil)
	content = llmutils.StringOrDefault(content, "Task completed but no final response was generated.")

	return content, nil
}

func (sm *SubagentManager) announceResult(
	label, task, result, status string,
	originChannel bus.Channel,
	originChatID string,
) {
	content := fmt.Sprintf(`[Subagent '%s' %s]

Task: %s

Result:
%s

Summarize this naturally for the user. Keep it brief (1-2 sentences). Do not mention technical details like "subagent" or task IDs.`,
		label, status, task, result)

	sm.bus.Publish(
		bus.NewAgentBusMessage(bus.ChannelSystem, bus.SenderIdSubAgent, string(originChannel)+":"+originChatID, content, ""),
	)
}

// shortID generates a short pseudo-unique ID (first 8 chars of a UUID-like value).
func shortID() string {
	return fmt.Sprintf("%08x", time.Now().UnixNano()&0xFFFFFFFF)
}

// SubAgent handles a single background task.
// It carries a restricted tool set (no spawn/message/cron) and starts fresh
// with no session history.
// Constructed per spawn call by AgentFactory.NewSubAgent().
type SubAgent struct {
	LoopRunner
	tools     tools.ToolList // value copy of restricted registry — no MCP tools
	workspace string
}

// Execute implements schema.Agent.
func (a *SubAgent) Execute(ctx context.Context, conversation schema.Messages, onProgress func(string)) (string, []string) {
	return a.run(ctx, conversation, &a.tools, onProgress)
}

func (agent *SubAgent) buildSystemPrompt() string {
	now := time.Now().Format("2006-01-02 15:04 (Monday)")
	tz, _ := time.Now().Zone()
	if tz == "" {
		tz = "UTC"
	}

	ws := expandHome(agent.workspace)

	goos := runtime.GOOS
	if goos == "darwin" {
		goos = "macOS"
	}

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
