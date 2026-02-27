package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/crystaldolphin/crystaldolphin/internal/bus"
	"github.com/crystaldolphin/crystaldolphin/internal/mcp"
	"github.com/crystaldolphin/crystaldolphin/internal/schema"
	"github.com/crystaldolphin/crystaldolphin/internal/session"
	"github.com/crystaldolphin/crystaldolphin/internal/shared/llmutils"
	"github.com/crystaldolphin/crystaldolphin/internal/tools"
)

// AgentLoop is the core processing engine.
//
// It reads InboundMessages from the bus, runs the LLM ↔ tool loop, and
// publishes OutboundMessages.  Each inbound message is handled in its own
// goroutine.
type AgentLoop struct {
	bus      bus.Bus
	provider schema.LLMProvider
	settings schema.AgentSettings

	agentContext *AgentContextBuilder
	sessions     *session.Manager
	compactor    schema.MemoryCompactor
	tools        tools.ToolList
	subagents    *SubagentManager

	// MCP server manager (connects at most once on first message).
	mcpManager *mcp.Manager
}

// NewAgentLoop creates an AgentLoop with the supplied tool registry builder and
// subagent manager. builtinSkillsDir may be "" if there are no embedded skills.
func NewAgentLoop(
	bus bus.Bus,
	provider schema.LLMProvider,
	settings schema.AgentSettings,
	mcpManager *mcp.Manager,
	sessions *session.Manager,
	compactor schema.MemoryCompactor,
	registry *tools.Registry,
	subagents *SubagentManager,
	ctxBuilder *AgentContextBuilder,
) *AgentLoop {
	return &AgentLoop{
		bus:          bus,
		provider:     provider,
		settings:     settings,
		mcpManager:   mcpManager,
		agentContext: ctxBuilder,
		sessions:     sessions,
		compactor:    compactor,
		tools:        registry.GetAll(),
		subagents:    subagents,
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
			if loop.mcpManager != nil {
				loop.mcpManager.Close()
			}
			return ctx.Err()
		}
	}
}

// ProcessDirect handles a message outside the bus (CLI, cron).
// Returns the final text response.
func (loop *AgentLoop) ProcessDirect(ctx context.Context, content, sessKey, channel, chatID string) string {
	loop.mcpManager.ConnectOnce(ctx, &loop.tools)

	msg := bus.NewInboundMessage(channel, "user", chatID, content)
	resp := loop.processMessage(ctx, msg, sessKey)
	if resp == nil {
		return ""
	}

	return resp.Content()
}

func (loop *AgentLoop) handleMessage(ctx context.Context, msg bus.InboundMessage) {
	loop.mcpManager.ConnectOnce(ctx, &loop.tools)
	resp := loop.processMessage(ctx, msg, "")

	if resp != nil {
		loop.bus.PublishOutbound(*resp)
	} else if msg.Channel() == "cli" {
		// Signal CLI that we're done even when MessageTool was used.
		out := bus.NewOutboundMessage(msg.Channel(), msg.ChatId(), "")
		out.SetMetadata(msg.Metadata())
		loop.bus.PublishOutbound(out)
	}
}

func (loop *AgentLoop) processMessage(ctx context.Context, msg bus.InboundMessage, sessionKeyOverride string) *bus.OutboundMessage {
	// System messages are injected by subagents.
	if msg.Channel() == "system" {
		return loop.handleSystemMessage(ctx, msg)
	}

	slog.Info(
		"Processing message",
		"channel", msg.Channel(),
		"sender", msg.SenderId(),
		"content", msg.ContentPreview(),
	)

	key := sessionKeyOverride
	if key == "" {
		key = msg.SessionKey()
	}

	ses := loop.sessions.GetOrCreate(key)

	if resp := loop.handleSlashCommand(msg, ses, key); resp != nil {
		return resp
	}

	loop.compactor.Schedule(key, ses, false)

	ctx, msgSentChan := loop.withTurnContext(ctx, msg)

	conversation := loop.agentContext.BuildMessages(
		ses.History(loop.settings.MemoryWindow),
		msg.Content(),
		msg.Media(),
		msg.Channel(),
		msg.ChatId(),
	)

	final, toolsUsed := loop.runLoop(ctx, conversation, loop.makeProgressCallback(msg))
	if final == "" {
		final = "I've completed processing but have no response to give."
	}

	slog.Info("Response", "channel", msg.Channel(), "sender", msg.SenderId(), "length", len(final))

	ses.AddUser(msg.Content())
	ses.AddAssistant(final, toolsUsed)
	loop.sessions.Save(ses)

	// If the message tool sent something, suppress the automatic reply.
	select {
	case <-msgSentChan:
		return nil
	default:
	}

	out := bus.NewOutboundMessage(msg.Channel(), msg.ChatId(), final)
	out.SetMetadata(msg.Metadata())
	return &out
}

// handleSlashCommand checks msg.Content for a known slash command and handles
// it. Returns non-nil if the command was handled (caller should return early).
func (loop *AgentLoop) handleSlashCommand(
	msg bus.InboundMessage,
	ses *session.SessionImpl,
	key string,
) *bus.OutboundMessage {
	cmd := strings.TrimSpace(strings.ToLower(msg.Content()))
	switch cmd {
	case "/new":
		return loop.handleCmdNew(msg, ses, key)
	case "/help":
		return loop.handleCmdHelp(msg)
	}
	return nil
}

// handleCmdNew clears the current session and triggers background memory
// consolidation, then replies with a confirmation.
func (loop *AgentLoop) handleCmdNew(msg bus.InboundMessage, sess *session.SessionImpl, key string) *bus.OutboundMessage {
	archived := sess.Messages()
	sess.Clear()
	loop.sessions.Save(sess)
	loop.sessions.Invalidate(key)

	tmp := session.NewArchivedSession(key, archived)
	loop.compactor.Schedule(key+":archive", tmp, true)

	out := bus.NewOutboundMessage(msg.Channel(), msg.ChatId(), "New session started. Memory consolidation in progress.")
	out.SetMetadata(msg.Metadata())

	return &out
}

// handleCmdHelp returns the help text listing available slash commands.
func (loop *AgentLoop) handleCmdHelp(msg bus.InboundMessage) *bus.OutboundMessage {
	out := bus.NewOutboundMessage(msg.Channel(), msg.ChatId(), "crystaldolphin commands:\n/new — Start a new conversation\n/help — Show available commands")
	out.SetMetadata(msg.Metadata())
	return &out
}

// withTurnContext decorates ctx with per-turn routing information and returns
// a channel that is closed when the message tool has sent a reply.
func (loop *AgentLoop) withTurnContext(ctx context.Context, msg bus.InboundMessage) (context.Context, chan struct{}) {
	msgID := ""
	if v, ok := msg.Metadata()["message_id"].(string); ok {
		msgID = v
	}
	msgSent := make(chan struct{})
	ctx = tools.WithTurn(ctx, tools.TurnContext{
		Channel:     msg.Channel(),
		ChatID:      msg.ChatId(),
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
		out := bus.NewOutboundMessage(msg.Channel(), msg.ChatId(), content)
		out.SetMetadata(meta)
		loop.bus.PublishOutbound(out)
	}
}

func (loop *AgentLoop) handleSystemMessage(ctx context.Context, msg bus.InboundMessage) *bus.OutboundMessage {
	channel, chatId, _ := strings.Cut(msg.ChatId(), ":")
	if chatId == "" {
		channel = "cli"
		chatId = msg.ChatId()
	}

	slog.Info("Processing system message", "sender", msg.SenderId())

	key := channel + ":" + chatId
	sess := loop.sessions.GetOrCreate(key)

	ctx = tools.WithTurn(ctx, tools.TurnContext{Channel: channel, ChatID: chatId})
	conversation := loop.agentContext.BuildMessages(
		sess.History(loop.settings.MemoryWindow),
		msg.Content(),
		nil,
		channel,
		chatId,
	)

	finalContent, _ := loop.runLoop(ctx, conversation, nil)
	if finalContent == "" {
		finalContent = "Background task completed."
	}

	sess.AddUser(fmt.Sprintf("[System: %s] %s", msg.SenderId(), msg.Content()))
	sess.AddAssistant(finalContent, nil)
	loop.sessions.Save(sess)

	out := bus.NewOutboundMessage(channel, chatId, finalContent)
	return &out
}

func (loop *AgentLoop) runLoop(ctx context.Context, conversation schema.Messages, onProgress func(string)) (finalContent string, toolsUsed []string) {
	for i := 0; i < loop.settings.MaxIter; i++ {
		resp, err := loop.provider.Chat(ctx,
			conversation,
			loop.tools.Definitions(),
			schema.NewChatOptions(loop.settings.Model, loop.settings.MaxTokens, loop.settings.Temperature),
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
			return llmutils.StripThink(content), toolsUsed
		}

		// Progress: emit partial text + tool hint.
		if onProgress != nil {
			if resp.Content != nil {
				if clean := llmutils.StripThink(*resp.Content); clean != "" {
					onProgress(clean)
				}
			}
			onProgress(llmutils.ToolHint(resp.ToolCalls))
		}

		// Append assistant turn with tool calls.
		var toolCalls []schema.ToolCall
		for _, tc := range resp.ToolCalls {
			toolCalls = append(toolCalls, schema.ToolCall{ID: tc.Id, Name: tc.Name, Arguments: tc.Arguments})
		}

		conversation.AddAssistant(resp.Content, toolCalls, resp.ReasoningContent)

		// Execute each tool.
		for _, tc := range resp.ToolCalls {
			toolsUsed = append(toolsUsed, tc.Name)
			argsJSON, _ := json.Marshal(tc.Arguments)

			slog.Info("Tool call", "name", tc.Name, "args", llmutils.Truncate(string(argsJSON), 200))

			var result string
			if t := loop.tools.Get(tc.Name); t != nil {
				result, _ = t.Execute(ctx, tc.Arguments)
			} else {
				result = fmt.Sprintf("Error: Tool '%s' not found", tc.Name)
			}

			conversation.AddToolResult(tc.Id, tc.Name, result)
		}
	}

	return "I've reached the maximum number of tool iterations without a final answer.", toolsUsed
}
