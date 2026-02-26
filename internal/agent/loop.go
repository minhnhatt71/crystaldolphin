package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"

	"github.com/crystaldolphin/crystaldolphin/internal/bus"
	"github.com/crystaldolphin/crystaldolphin/internal/config"
	"github.com/crystaldolphin/crystaldolphin/internal/schema"
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
	bus      bus.Bus
	provider schema.LLMProvider
	cfg      *config.Config

	model        string
	maxIter      int
	temperature  float64
	maxTokens    int
	memoryWindow int
	workspace    string

	agentContext *ContextBuilder
	sessions     *session.Manager
	tools        tools.ToolList

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
	messageBus bus.Bus,
	provider schema.LLMProvider,
	cfg *config.Config,
	sessions *session.Manager,
	registry *tools.Registry,
	subagents *SubagentManager,
	ctxBuilder *ContextBuilder,
) *AgentLoop {
	workspace := cfg.WorkspacePath()
	model := cfg.Agents.Defaults.Model
	if model == "" {
		model = provider.DefaultModel()
	}

	return &AgentLoop{
		bus:           messageBus,
		provider:      provider,
		cfg:           cfg,
		model:         model,
		maxIter:       cfg.Agents.Defaults.MaxToolIter,
		temperature:   cfg.Agents.Defaults.Temperature,
		maxTokens:     cfg.Agents.Defaults.MaxTokens,
		memoryWindow:  cfg.Agents.Defaults.MemoryWindow,
		workspace:     workspace,
		agentContext:  ctxBuilder,
		sessions:      sessions,
		tools:         registry.GetAll(),
		subagents:     subagents,
		consolidating: make(map[string]bool),
	}
}

// Run reads from the inbound bus and processes each message in a goroutine.
// Blocks until ctx is cancelled.
func (loop *AgentLoop) Run(ctx context.Context) error {
	// Connect MCP servers once, lazily on first message.
	slog.Info("Agent loop started")

	for {
		select {
		case msg := <-loop.bus.SubscribeInbound():
			go loop.handleMessage(ctx, msg)
		case <-ctx.Done():
			slog.Info("Agent loop stopping")
			if loop.mcpCleanup != nil {
				loop.mcpCleanup()
			}
			return ctx.Err()
		}
	}
}

// ProcessDirect handles a message outside the bus (CLI, cron).
// Returns the final text response.
func (loop *AgentLoop) ProcessDirect(
	ctx context.Context,
	content, sessionKey, channel, chatID string,
) string {
	loop.connectMCPOnce(ctx)
	msg := bus.NewInboundMessage(channel, "user", chatID, content)
	resp := loop.processMessage(ctx, msg, sessionKey)
	if resp == nil {
		return ""
	}
	return resp.Content()
}

func (loop *AgentLoop) handleMessage(ctx context.Context, msg bus.InboundMessage) {
	loop.connectMCPOnce(ctx)
	resp := loop.processMessage(ctx, msg, "")
	if resp != nil {
		loop.bus.PublishOutbound(*resp)
	} else if msg.Channel() == "cli" {
		// Signal CLI that we're done even when MessageTool was used.
		out := bus.NewOutboundMessage(msg.Channel(), msg.ChatID(), "")
		out.SetMetadata(msg.Metadata())
		loop.bus.PublishOutbound(out)
	}
}

func (loop *AgentLoop) processMessage(
	ctx context.Context,
	msg bus.InboundMessage,
	sessionKeyOverride string,
) *bus.OutboundMessage {
	// System messages are injected by subagents.
	if msg.Channel() == "system" {
		return loop.handleSystemMessage(ctx, msg)
	}

	slog.Info("Processing message",
		"channel", msg.Channel(),
		"sender", msg.SenderID(),
		"content", msg.ContentPreview())

	key := sessionKeyOverride
	if key == "" {
		key = msg.SessionKey()
	}

	sess := loop.sessions.GetOrCreate(key)

	// Slash commands.
	if resp := loop.handleSlashCommand(ctx, msg, sess, key); resp != nil {
		return resp
	}

	loop.maybeConsolidateBackground(key, sess)

	ctx, msgSent := loop.injectTurnContext(ctx, msg)

	history := loop.agentContext.BuildMessages(
		sess.GetHistory(loop.memoryWindow),
		msg.Content(),
		msg.Media(),
		msg.Channel(), msg.ChatID(),
	)

	onProgress := loop.makeProgressCallback(msg)

	finalContent, toolsUsed := loop.runLoop(ctx, history, onProgress)
	if finalContent == "" {
		finalContent = "I've completed processing but have no response to give."
	}

	slog.Info("Response", "channel", msg.Channel(), "sender", msg.SenderID(), "length", len(finalContent))

	sess.AddUser(msg.Content())
	sess.AddAssistant(finalContent, toolsUsed)
	loop.sessions.Save(sess)

	// If the message tool sent something, suppress the automatic reply.
	select {
	case <-msgSent:
		return nil
	default:
	}

	out := bus.NewOutboundMessage(msg.Channel(), msg.ChatID(), finalContent)
	out.SetMetadata(msg.Metadata())
	return &out
}

// handleSlashCommand checks msg.Content for a known slash command and handles
// it. Returns non-nil if the command was handled (caller should return early).
func (loop *AgentLoop) handleSlashCommand(
	ctx context.Context,
	msg bus.InboundMessage,
	sess *session.Session,
	key string,
) *bus.OutboundMessage {
	cmd := strings.TrimSpace(strings.ToLower(msg.Content()))
	switch cmd {
	case "/new":
		return loop.handleCmdNew(ctx, msg, sess, key)
	case "/help":
		return loop.handleCmdHelp(msg)
	}
	return nil
}

// handleCmdNew clears the current session and triggers background memory
// consolidation, then replies with a confirmation.
func (loop *AgentLoop) handleCmdNew(
	ctx context.Context,
	msg bus.InboundMessage,
	sess *session.Session,
	key string,
) *bus.OutboundMessage {
	archived := sess.Messages
	sess.Clear()
	loop.sessions.Save(sess)
	loop.sessions.Invalidate(key)

	go func() {
		tmp := &session.Session{Key: key, Messages: archived}
		mem, err := NewMemoryStore(loop.workspace)
		if err != nil {
			slog.Error("Failed to create memory store for consolidation", "err", err)
			return
		}
		err = mem.Consolidate(ctx, tmp, loop.sessions, loop.provider, loop.model, true, loop.memoryWindow)
		if err != nil {
			slog.Error("Memory consolidation failed", "err", err)
		}
	}()

	out := bus.NewOutboundMessage(msg.Channel(), msg.ChatID(), "New session started. Memory consolidation in progress.")
	out.SetMetadata(msg.Metadata())
	return &out
}

// handleCmdHelp returns the help text listing available slash commands.
func (loop *AgentLoop) handleCmdHelp(msg bus.InboundMessage) *bus.OutboundMessage {
	out := bus.NewOutboundMessage(msg.Channel(), msg.ChatID(), "crystaldolphin commands:\n/new — Start a new conversation\n/help — Show available commands")
	out.SetMetadata(msg.Metadata())
	return &out
}

// maybeConsolidateBackground triggers a background memory consolidation when
// the session history exceeds memoryWindow. Guards against duplicate runs with
// the per-key consolidating flag.
func (loop *AgentLoop) maybeConsolidateBackground(key string, sess *session.Session) {
	if sess.Len() <= loop.memoryWindow {
		return
	}
	loop.consolidatingMu.Lock()
	defer loop.consolidatingMu.Unlock()
	if loop.consolidating[key] {
		return
	}
	loop.consolidating[key] = true
	go func() {
		defer func() {
			loop.consolidatingMu.Lock()
			delete(loop.consolidating, key)
			loop.consolidatingMu.Unlock()
		}()
		mem, err := NewMemoryStore(loop.workspace)
		if err != nil {
			slog.Error("Failed to create memory store for consolidation", "err", err)
			return
		}
		err = mem.Consolidate(context.Background(), sess, loop.sessions, loop.provider, loop.model, false, loop.memoryWindow)
		if err != nil {
			slog.Error("Memory consolidation failed", "err", err)
		}
	}()
}

// injectTurnContext decorates ctx with per-turn routing information and returns
// a channel that is closed when the message tool has sent a reply.
func (loop *AgentLoop) injectTurnContext(ctx context.Context, msg bus.InboundMessage) (context.Context, chan struct{}) {
	msgID := ""
	if v, ok := msg.Metadata()["message_id"].(string); ok {
		msgID = v
	}
	msgSent := make(chan struct{})
	ctx = tools.WithTurn(ctx, tools.TurnContext{
		Channel:     msg.Channel(),
		ChatID:      msg.ChatID(),
		MsgID:       msgID,
		MessageSent: msgSent,
	})
	return ctx, msgSent
}

// makeProgressCallback returns a function that pushes intermediate output to
// the outbound bus so clients can display streaming progress.
func (loop *AgentLoop) makeProgressCallback(msg bus.InboundMessage) func(string) {
	return func(content string) {
		meta := map[string]any{"_progress": true}
		for k, v := range msg.Metadata() {
			meta[k] = v
		}
		out := bus.NewOutboundMessage(msg.Channel(), msg.ChatID(), content)
		out.SetMetadata(meta)
		loop.bus.PublishOutbound(out)
	}
}

func (loop *AgentLoop) handleSystemMessage(ctx context.Context, msg bus.InboundMessage) *bus.OutboundMessage {
	channel, chatID, _ := strings.Cut(msg.ChatID(), ":")
	if chatID == "" {
		channel = "cli"
		chatID = msg.ChatID()
	}

	slog.Info("Processing system message", "sender", msg.SenderID())

	key := channel + ":" + chatID
	sess := loop.sessions.GetOrCreate(key)

	ctx = tools.WithTurn(ctx, tools.TurnContext{Channel: channel, ChatID: chatID})
	conversation := loop.agentContext.BuildMessages(
		sess.GetHistory(loop.memoryWindow),
		msg.Content(),
		nil,
		channel,
		chatID,
	)

	finalContent, _ := loop.runLoop(ctx, conversation, nil)
	if finalContent == "" {
		finalContent = "Background task completed."
	}

	sess.AddUser(fmt.Sprintf("[System: %s] %s", msg.SenderID(), msg.Content()))
	sess.AddAssistant(finalContent, nil)
	loop.sessions.Save(sess)

	out := bus.NewOutboundMessage(channel, chatID, finalContent)
	return &out
}

func (loop *AgentLoop) runLoop(ctx context.Context, conversation schema.Messages, onProgress func(string)) (finalContent string, toolsUsed []string) {
	for i := 0; i < loop.maxIter; i++ {
		resp, err := loop.provider.Chat(ctx,
			conversation,
			loop.tools.Definitions(),
			schema.NewChatOptions(loop.model, loop.maxTokens, loop.temperature),
		)

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
		var toolCalls []schema.ToolCall
		for _, tc := range resp.ToolCalls {
			toolCalls = append(toolCalls, schema.ToolCall{ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments})
		}
		conversation.AddAssistant(resp.Content, toolCalls, resp.ReasoningContent)

		// Execute each tool.
		for _, tc := range resp.ToolCalls {
			toolsUsed = append(toolsUsed, tc.Name)
			argsJSON, _ := json.Marshal(tc.Arguments)

			slog.Info("Tool call", "name", tc.Name, "args", truncate(string(argsJSON), 200))

			var result string
			if t := loop.tools.Get(tc.Name); t != nil {
				result, _ = t.Execute(ctx, tc.Arguments)
			} else {
				result = fmt.Sprintf("Error: Tool '%s' not found", tc.Name)
			}

			conversation.AddToolResult(tc.ID, tc.Name, result)
		}
	}

	return "I've reached the maximum number of tool iterations without a final answer.", toolsUsed
}

// connectMCPOnce connects to MCP servers the first time it is called.
func (loop *AgentLoop) connectMCPOnce(ctx context.Context) {
	loop.mcpOnce.Do(func() {
		if len(loop.cfg.Tools.MCPServers) == 0 {
			return
		}
		// Convert config.MCPServerConfig → tools.MCPServerConfig
		servers := make(map[string]tools.MCPServerConfig, len(loop.cfg.Tools.MCPServers))
		for name, c := range loop.cfg.Tools.MCPServers {
			servers[name] = tools.MCPServerConfig{
				Command: c.Command,
				Args:    c.Args,
				Env:     c.Env,
				URL:     c.URL,
				Headers: c.Headers,
			}
		}
		loop.mcpCleanup = tools.ConnectMCPServers(ctx, servers, &loop.tools)
	})
}

// stripThink removes <think>…</think> blocks that some models embed.
func stripThink(s string) string {
	return strings.TrimSpace(reThink.ReplaceAllString(s, ""))
}

// toolHint formats tool calls as a concise hint string, e.g. web_search("query").
func toolHint(tcs []schema.ToolCallRequest) string {
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
