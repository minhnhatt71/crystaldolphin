package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/crystaldolphin/crystaldolphin/internal/bus"
	"github.com/crystaldolphin/crystaldolphin/internal/config"
	"github.com/crystaldolphin/crystaldolphin/internal/providers"
	"github.com/crystaldolphin/crystaldolphin/internal/session"
	"github.com/crystaldolphin/crystaldolphin/internal/tools"
)

var reThink = regexp.MustCompile(`(?s)<think>.*?</think>`)

// AgentLoop is the core processing engine.
//
// It reads InboundMessages from the bus, runs the LLM ↔ tool loop, and
// publishes OutboundMessages.  Each inbound message is handled in its own
// goroutine.
type AgentLoop struct {
	bus      *bus.MessageBus
	provider providers.LLMProvider
	cfg      *config.Config

	model            string
	maxIter          int
	temperature      float64
	maxTokens        int
	memoryWindow     int
	workspace        string
	builtinSkillsDir string

	ctx      *ContextBuilder
	sessions *session.Manager
	tools    tools.ToolList

	subagents *SubagentManager

	// Per-session consolidation guard (string set).
	consolidating   map[string]bool
	consolidatingMu sync.Mutex

	// MCP cleanup.
	mcpCleanup func()
	mcpOnce    sync.Once
}

// NewAgentLoop creates an AgentLoop with the supplied tool registry builder and
// subagent manager. builtinSkillsDir may be "" if there are no embedded skills.
func NewAgentLoop(
	b *bus.MessageBus,
	provider providers.LLMProvider,
	cfg *config.Config,
	registry *tools.Registry,
	subagents *SubagentManager,
	builtinSkillsDir string,
) *AgentLoop {
	workspace := cfg.WorkspacePath()
	model := cfg.Agents.Defaults.Model
	if model == "" {
		model = provider.DefaultModel()
	}

	toolList := tools.NewToolList([]tools.Tool{
		registry.Get(tools.ToolReadFile),
		registry.Get(tools.ToolWriteFile),
		registry.Get(tools.ToolEditFile),
		registry.Get(tools.ToolListDir),
		registry.Get(tools.ToolExec),
		registry.Get(tools.ToolWebSearch),
		registry.Get(tools.ToolWebFetch),
		registry.Get(tools.ToolMessage),
		registry.Get(tools.ToolSpawn),
		registry.Get(tools.ToolCron),
	})

	return &AgentLoop{
		bus:              b,
		provider:         provider,
		cfg:              cfg,
		model:            model,
		maxIter:          cfg.Agents.Defaults.MaxToolIter,
		temperature:      cfg.Agents.Defaults.Temperature,
		maxTokens:        cfg.Agents.Defaults.MaxTokens,
		memoryWindow:     cfg.Agents.Defaults.MemoryWindow,
		workspace:        workspace,
		builtinSkillsDir: builtinSkillsDir,
		ctx:              NewContextBuilder(workspace, builtinSkillsDir),
		sessions:         mustNewSessionManager(workspace),
		tools:            toolList,
		subagents:        subagents,
		consolidating:    make(map[string]bool),
	}
}

// Run reads from the inbound bus and processes each message in a goroutine.
// Blocks until ctx is cancelled.
func (al *AgentLoop) Run(ctx context.Context) error {
	// Connect MCP servers once, lazily on first message.
	slog.Info("Agent loop started")

	for {
		select {
		case msg := <-al.bus.Inbound:
			go al.handleMessage(ctx, msg)
		case <-ctx.Done():
			slog.Info("Agent loop stopping")
			if al.mcpCleanup != nil {
				al.mcpCleanup()
			}
			return ctx.Err()
		}
	}
}

// ProcessDirect handles a message outside the bus (CLI, cron).
// Returns the final text response.
func (al *AgentLoop) ProcessDirect(
	ctx context.Context,
	content, sessionKey, channel, chatID string,
) string {
	al.connectMCPOnce(ctx)
	msg := bus.InboundMessage{
		Channel:   channel,
		SenderID:  "user",
		ChatID:    chatID,
		Content:   content,
		Timestamp: time.Now(),
	}
	resp := al.processMessage(ctx, msg, sessionKey)
	if resp == nil {
		return ""
	}
	return resp.Content
}

// ---------------------------------------------------------------------------
// Message processing
// ---------------------------------------------------------------------------

func (al *AgentLoop) handleMessage(ctx context.Context, msg bus.InboundMessage) {
	al.connectMCPOnce(ctx)
	resp := al.processMessage(ctx, msg, "")
	if resp != nil {
		al.bus.Outbound <- *resp
	} else if msg.Channel == "cli" {
		// Signal CLI that we're done even when MessageTool was used.
		al.bus.Outbound <- bus.OutboundMessage{
			Channel:  msg.Channel,
			ChatID:   msg.ChatID,
			Content:  "",
			Metadata: msg.Metadata,
		}
	}
}

func (al *AgentLoop) processMessage(
	ctx context.Context,
	msg bus.InboundMessage,
	sessionKeyOverride string,
) *bus.OutboundMessage {
	// System messages are injected by subagents.
	if msg.Channel == "system" {
		return al.handleSystemMessage(ctx, msg)
	}

	preview := msg.Content
	if len(preview) > 80 {
		preview = preview[:80] + "..."
	}
	slog.Info("Processing message", "channel", msg.Channel, "sender", msg.SenderID, "content", preview)

	key := sessionKeyOverride
	if key == "" {
		key = msg.SessionKey()
	}
	sess := al.sessions.GetOrCreate(key)

	// Slash commands.
	cmd := strings.TrimSpace(strings.ToLower(msg.Content))
	switch cmd {
	case "/new":
		archived := make([]map[string]any, len(sess.Messages))
		copy(archived, sess.Messages)
		sess.Clear()
		al.sessions.Save(sess)
		al.sessions.Invalidate(key)

		go func() {
			tmp := &session.Session{Key: key}
			tmp.Messages = archived
			mem, _ := NewMemoryStore(al.workspace)
			if mem != nil {
				_ = mem.Consolidate(ctx, tmp, al.provider, al.model, true, al.memoryWindow)
			}
		}()
		return &bus.OutboundMessage{
			Channel:  msg.Channel,
			ChatID:   msg.ChatID,
			Content:  "New session started. Memory consolidation in progress.",
			Metadata: msg.Metadata,
		}
	case "/help":
		return &bus.OutboundMessage{
			Channel:  msg.Channel,
			ChatID:   msg.ChatID,
			Content:  "crystaldolphin commands:\n/new — Start a new conversation\n/help — Show available commands",
			Metadata: msg.Metadata,
		}
	}

	// Trigger background consolidation when history is long.
	if len(sess.Messages) > al.memoryWindow {
		al.consolidatingMu.Lock()
		if !al.consolidating[key] {
			al.consolidating[key] = true
			go func() {
				defer func() {
					al.consolidatingMu.Lock()
					delete(al.consolidating, key)
					al.consolidatingMu.Unlock()
				}()
				mem, _ := NewMemoryStore(al.workspace)
				if mem != nil {
					_ = mem.Consolidate(context.Background(), sess, al.provider, al.model, false, al.memoryWindow)
				}
			}()
		}
		al.consolidatingMu.Unlock()
	}

	// Inject per-turn context into stateful tools.
	msgID := ""
	if v, ok := msg.Metadata["message_id"].(string); ok {
		msgID = v
	}
	al.setToolContext(msg.Channel, msg.ChatID, msgID)

	// Reset message tool send tracking.
	if t := al.tools.Get("message"); t != nil {
		if mt, ok := t.(*tools.MessageTool); ok {
			mt.StartTurn()
		}
	}

	history := al.ctx.BuildMessages(
		sess.GetHistory(al.memoryWindow),
		msg.Content,
		msg.Media,
		msg.Channel, msg.ChatID,
	)

	// Progress callback — push intermediate output to the bus.
	onProgress := func(content string) {
		meta := map[string]any{"_progress": true}
		for k, v := range msg.Metadata {
			meta[k] = v
		}
		al.bus.Outbound <- bus.OutboundMessage{
			Channel:  msg.Channel,
			ChatID:   msg.ChatID,
			Content:  content,
			Metadata: meta,
		}
	}

	finalContent, toolsUsed := al.runLoop(ctx, history, onProgress)
	if finalContent == "" {
		finalContent = "I've completed processing but have no response to give."
	}

	slog.Info("Response", "channel", msg.Channel, "sender", msg.SenderID, "length", len(finalContent))

	// Persist to session.
	sess.AddMessage("user", msg.Content, nil)
	extras := map[string]any{}
	if len(toolsUsed) > 0 {
		extras["tools_used"] = toolsUsed
	}
	sess.AddMessage("assistant", finalContent, extras)
	al.sessions.Save(sess)

	// If the message tool sent something, suppress the return message.
	if t := al.tools.Get("message"); t != nil {
		if mt, ok := t.(*tools.MessageTool); ok && mt.WasSentInTurn() {
			return nil
		}
	}

	return &bus.OutboundMessage{
		Channel:  msg.Channel,
		ChatID:   msg.ChatID,
		Content:  finalContent,
		Metadata: msg.Metadata,
	}
}

func (al *AgentLoop) handleSystemMessage(ctx context.Context, msg bus.InboundMessage) *bus.OutboundMessage {
	channel, chatID, _ := strings.Cut(msg.ChatID, ":")
	if chatID == "" {
		channel = "cli"
		chatID = msg.ChatID
	}
	slog.Info("Processing system message", "sender", msg.SenderID)

	key := channel + ":" + chatID
	session := al.sessions.GetOrCreate(key)

	al.setToolContext(channel, chatID, "")
	conversation := al.ctx.BuildMessages(
		session.GetHistory(al.memoryWindow),
		msg.Content,
		nil,
		channel, chatID,
	)

	finalContent, _ := al.runLoop(ctx, conversation, nil)
	if finalContent == "" {
		finalContent = "Background task completed."
	}

	session.AddMessage("user", fmt.Sprintf("[System: %s] %s", msg.SenderID, msg.Content), nil)
	session.AddMessage("assistant", finalContent, nil)
	al.sessions.Save(session)

	return &bus.OutboundMessage{
		Channel: channel,
		ChatID:  chatID,
		Content: finalContent,
	}
}

func (al *AgentLoop) runLoop(ctx context.Context, messages MessageHistory, onProgress func(string)) (finalContent string, toolsUsed []string) {
	for i := 0; i < al.maxIter; i++ {
		resp, err := al.provider.Chat(ctx, messages, al.tools.Definitions(), providers.ChatOptions{
			Model:       al.model,
			MaxTokens:   al.maxTokens,
			Temperature: al.temperature,
		})

		if err != nil {
			slog.Error("LLM error", "err", err)
			return "Sorry, I encountered an error calling the LLM.", nil
		}

		if len(resp.ToolCalls) == 0 {
			// Terminal response.
			content := ""
			if resp.Content != nil {
				content = *resp.Content
			}
			return stripThink(content), toolsUsed
		}

		// Progress: emit partial text + tool hint.
		if onProgress != nil {
			if resp.Content != nil {
				if clean := stripThink(*resp.Content); clean != "" {
					onProgress(clean)
				}
			}
			onProgress(toolHint(resp.ToolCalls))
		}

		// Append assistant turn with tool calls.
		var toolCalls []ToolCallDict
		for _, tc := range resp.ToolCalls {
			toolCalls = append(toolCalls, ToolCallDict{ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments})
		}
		messages.AddAssistant(resp.Content, toolCalls, resp.ReasoningContent)

		// Execute each tool.
		for _, tc := range resp.ToolCalls {
			toolsUsed = append(toolsUsed, tc.Name)
			argsJSON, _ := json.Marshal(tc.Arguments)
			slog.Info("Tool call", "name", tc.Name, "args", truncate(string(argsJSON), 200))
			result, _ := al.tools.Get(tc.Name).Execute(ctx, tc.Arguments)
			messages.AddToolResult(tc.ID, tc.Name, result)
		}
	}

	return "I've reached the maximum number of tool iterations without a final answer.", toolsUsed
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// setToolContext injects routing context into stateful tools.
func (al *AgentLoop) setToolContext(channel, chatID, msgID string) {
	if t := al.tools.Get("message"); t != nil {
		if mt, ok := t.(*tools.MessageTool); ok {
			mt.SetContext(channel, chatID, msgID)
		}
	}
	if t := al.tools.Get("spawn"); t != nil {
		if st, ok := t.(*tools.SpawnTool); ok {
			st.SetContext(channel, chatID)
		}
	}
	if t := al.tools.Get("cron"); t != nil {
		if ct, ok := t.(*tools.CronTool); ok {
			ct.SetContext(channel, chatID)
		}
	}
}

// connectMCPOnce connects to MCP servers the first time it is called.
func (al *AgentLoop) connectMCPOnce(ctx context.Context) {
	al.mcpOnce.Do(func() {
		if len(al.cfg.Tools.MCPServers) == 0 {
			return
		}
		// Convert config.MCPServerConfig → tools.MCPServerConfig
		servers := make(map[string]tools.MCPServerConfig, len(al.cfg.Tools.MCPServers))
		for name, c := range al.cfg.Tools.MCPServers {
			servers[name] = tools.MCPServerConfig{
				Command: c.Command,
				Args:    c.Args,
				Env:     c.Env,
				URL:     c.URL,
				Headers: c.Headers,
			}
		}
		al.mcpCleanup = tools.ConnectMCPServers(ctx, servers, &al.tools)
	})
}

// stripThink removes <think>…</think> blocks that some models embed.
func stripThink(s string) string {
	return strings.TrimSpace(reThink.ReplaceAllString(s, ""))
}

// toolHint formats tool calls as a concise hint string, e.g. web_search("query").
func toolHint(tcs []providers.ToolCallRequest) string {
	parts := make([]string, 0, len(tcs))
	for _, tc := range tcs {
		var firstVal string
		for _, v := range tc.Arguments {
			if s, ok := v.(string); ok {
				firstVal = s
			}
			break
		}
		if firstVal == "" {
			parts = append(parts, tc.Name)
			continue
		}
		if len(firstVal) > 40 {
			firstVal = firstVal[:40] + "…"
		}
		parts = append(parts, fmt.Sprintf("%s(%q)", tc.Name, firstVal))
	}
	return strings.Join(parts, ", ")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// mustNewSessionManager creates a session.Manager, panicking on error.
// Errors here mean the workspace directory is completely inaccessible, which
// is an unrecoverable condition at startup.
func mustNewSessionManager(workspace string) *session.Manager {
	m, err := session.NewManager(workspace)
	if err != nil {
		panic(fmt.Sprintf("failed to create session manager: %v", err))
	}
	return m
}
