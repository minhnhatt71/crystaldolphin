package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/crystaldolphin/crystaldolphin/internal/bus"
	"github.com/crystaldolphin/crystaldolphin/internal/schema"
	"github.com/crystaldolphin/crystaldolphin/internal/session"
	"github.com/crystaldolphin/crystaldolphin/internal/shared/llmutils"
	"github.com/crystaldolphin/crystaldolphin/internal/tools"
)

// AgentLoop is the core processing engine.
//
// It reads InboundMessages from the bus, routes each message to the
// appropriate channel-kind handler, and publishes OutboundMessages.
// Each inbound message is handled in its own goroutine.
type AgentLoop struct {
	bus      bus.Bus
	settings schema.AgentSettings

	promptBuilder *PromptContext
	sessions      *session.Manager
	compactor     schema.MemoryCompactor
	tools         tools.ToolList // MCP registration target; factory holds &loop.tools
	subagents     *SubagentManager

	runner  LoopRunner    // shared LLM iteration logic (used by handleSystemChannel)
	factory *AgentFactory // creates per-request CoreAgent / SubAgent instances
}

// NewAgentLoop creates an AgentLoop with the supplied factory, tool registry, and
// subagent manager.
func NewAgentLoop(
	bus bus.Bus,
	factory *AgentFactory,
	settings schema.AgentSettings,
	sessions *session.Manager,
	compactor schema.MemoryCompactor,
	registry *tools.Registry,
	subagents *SubagentManager,
	promptBuilder *PromptContext,
) *AgentLoop {
	loop := &AgentLoop{
		bus:           bus,
		settings:      settings,
		promptBuilder: promptBuilder,
		sessions:      sessions,
		compactor:     compactor,
		tools:         registry.GetAll(),
		subagents:     subagents,
		runner:        newLoopRunner(factory.provider, settings),
		factory:       factory,
	}
	// Wire the factory's coreTools pointer to this loop's live ToolList so that
	// MCP tools added via ConnectOnce are visible to every CoreAgent created by
	// the factory.
	factory.SetCoreTools(&loop.tools)
	return loop
}

// Run reads from the inbound bus and processes each message in a goroutine.
// Blocks until ctx is cancelled.
func (loop *AgentLoop) Run(ctx context.Context) error {
	slog.Info("Agent loop started")

	for {
		select {
		case msg := <-loop.bus.InboundChan():
			go loop.handleMessage(ctx, msg)
		case <-ctx.Done():
			slog.Info("Agent loop stopping")
			loop.factory.Close()
			return ctx.Err()
		}
	}
}

// ProcessDirect handles a message outside the bus (CLI, cron).
// Returns the final text response.
func (loop *AgentLoop) ProcessDirect(ctx context.Context, content, sessKey, channel, chatID string) string {
	msg := bus.NewInboundMessage(channel, "user", chatID, content)
	res := loop.processMessage(ctx, msg, sessKey)
	if res == nil {
		return ""
	}

	return res.Content()
}

func (loop *AgentLoop) handleMessage(ctx context.Context, msg bus.InboundMessage) {
	resp := loop.processMessage(ctx, msg, "")

	if resp != nil {
		loop.bus.PublishOutbound(*resp)
	} else if bus.ChannelType(msg.Channel()) == bus.ChannelCLI {
		// Signal CLI that we're done even when MessageTool was used.
		out := bus.NewOutboundMessage(msg.Channel(), msg.ChatId(), "")
		out.SetMetadata(msg.Metadata())
		loop.bus.PublishOutbound(out)
	}
}

// processMessage is a thin adapter kept for ProcessDirect compatibility.
func (loop *AgentLoop) processMessage(
	ctx context.Context,
	msg bus.InboundMessage,
	sessionKeyOverride string,
) *bus.OutboundMessage {
	return loop.routeMessage(ctx, msg, sessionKeyOverride)
}

// routeMessage dispatches msg to the appropriate channel-kind handler.
// sessionKeyOverride is non-empty only when called from ProcessDirect.
func (loop *AgentLoop) routeMessage(ctx context.Context, msg bus.InboundMessage, sessionKeyOverride string) *bus.OutboundMessage {
	switch bus.ChannelType(msg.Channel()) {
	case bus.ChannelSystem:
		return loop.handleSystemChannel(ctx, msg)
	case bus.ChannelCLI:
		return loop.handleCLIChannel(ctx, msg, sessionKeyOverride)
	case bus.ChannelCron:
		return loop.handleCronChannel(ctx, msg, sessionKeyOverride)
	case bus.ChannelHeartbeat:
		return loop.handleHeartbeatChannel(ctx, msg, sessionKeyOverride)
	default:
		return loop.handleExternalChannel(ctx, msg, sessionKeyOverride)
	}
}

// handleSystemChannel processes system-channel messages injected by subagents.
// It parses the original channel/chat from msg.ChatId, runs one LLM summarisation
// turn, and routes the reply to the original chat.
func (loop *AgentLoop) handleSystemChannel(ctx context.Context, msg bus.InboundMessage) *bus.OutboundMessage {
	channel, chatId, _ := strings.Cut(msg.ChatId(), ":")
	if chatId == "" {
		channel = "cli"
		chatId = msg.ChatId()
	}

	slog.Info("Processing system message", "sender", msg.SenderId())

	key := channel + ":" + chatId
	sess := loop.sessions.GetOrCreate(key)

	ctx = tools.WithTurn(ctx, tools.TurnContext{Channel: channel, ChatID: chatId})

	conversation := loop.promptBuilder.BuildMessages(
		sess.History(loop.settings.MemoryWindow),
		msg.Content(),
		nil,
		channel,
		chatId,
	)

	final, _ := loop.runner.run(ctx, conversation, &loop.tools, nil)
	final = llmutils.StringOrDefault(final, "Background task completed.")

	sess.AddUser(fmt.Sprintf("[System: %s] %s", msg.SenderId(), msg.Content()))
	sess.AddAssistant(final, nil)
	loop.sessions.Save(sess)

	out := bus.NewOutboundMessage(channel, chatId, final)
	return &out
}

// handleCLIChannel handles messages arriving on the CLI channel.
// The full pipeline is identical to external channels; the CLI-specific
// empty-outbound signal (when MessageTool fired) is handled in handleMessage.
func (loop *AgentLoop) handleCLIChannel(ctx context.Context, msg bus.InboundMessage, sessionKeyOverride string) *bus.OutboundMessage {
	return loop.handleExternalChannel(ctx, msg, sessionKeyOverride)
}

// handleCronChannel handles messages arriving on cron or heartbeat channels.
// These channels always use ProcessDirect (bypassing the bus); if a message
// somehow arrives on the bus the pipeline runs but no outbound is published.
func (loop *AgentLoop) handleCronChannel(ctx context.Context, msg bus.InboundMessage, sessionKeyOverride string) *bus.OutboundMessage {
	loop.handleExternalChannel(ctx, msg, sessionKeyOverride)
	return nil
}

func (loop *AgentLoop) handleHeartbeatChannel(ctx context.Context, msg bus.InboundMessage, sessionKeyOverride string) *bus.OutboundMessage {
	loop.handleExternalChannel(ctx, msg, sessionKeyOverride)
	return nil
}

// handleExternalChannel processes messages from external chat platforms
// (telegram, discord, slack, whatsapp, feishu, dingtalk, email, mochat, qq).
// It runs slash commands, the full LLM loop, saves the session, and returns
// an OutboundMessage — or nil if the message tool already sent the reply.
func (loop *AgentLoop) handleExternalChannel(ctx context.Context, msg bus.InboundMessage, sessionKeyOverride string) *bus.OutboundMessage {
	slog.Info(
		"Processing message",
		"sender", msg.SenderId(),
		"channel", msg.Channel(),
		"content", llmutils.Truncate(msg.Content(), 80),
	)

	key := llmutils.StringOrDefault(sessionKeyOverride, msg.SessionKey())
	ses := loop.sessions.GetOrCreate(key)

	if resp := loop.handleSlashCommand(msg, ses, key); resp != nil {
		return resp
	}

	loop.compactor.Schedule(key, ses, false)

	ctx, msgSentChan := loop.withTurnContext(ctx, msg)

	conversation := loop.promptBuilder.BuildMessages(
		ses.History(loop.settings.MemoryWindow),
		msg.Content(),
		msg.Media(),
		msg.Channel(),
		msg.ChatId(),
	)

	coreAgent := loop.factory.NewCoreAgent()
	final, toolsUsed := coreAgent.Execute(ctx, conversation, loop.makeProgressCallback(msg))
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
	ses *session.ChannelSessionImpl,
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
func (loop *AgentLoop) handleCmdNew(msg bus.InboundMessage, sess *session.ChannelSessionImpl, key string) *bus.OutboundMessage {
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
