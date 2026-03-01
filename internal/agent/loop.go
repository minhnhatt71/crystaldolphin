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
	agentBus   *bus.AgentBus
	channelBus *bus.ChannelBus
	consoleBus *bus.ConsoleBus
	settings   schema.AgentSettings
	pctx       *PromptContext
	sessions   *session.Manager
	compactor  schema.MemoryCompactor
	tools      tools.ToolList // MCP registration target; factory holds &loop.tools
	subagents  *SubagentManager

	runner  LoopRunner    // shared LLM iteration logic (used by handleSystemChannel)
	factory *AgentFactory // creates per-request CoreAgent / SubAgent instances
}

// NewAgentLoop creates an AgentLoop with the supplied factory, tool registry, and
// subagent manager.
func NewAgentLoop(
	agentBus *bus.AgentBus,
	channelBus *bus.ChannelBus,
	consoleBus *bus.ConsoleBus,
	factory *AgentFactory,
	settings schema.AgentSettings,
	sessions *session.Manager,
	compactor schema.MemoryCompactor,
	registry *tools.Registry,
	subagents *SubagentManager,
	promptBuilder *PromptContext,
) *AgentLoop {
	loop := &AgentLoop{
		agentBus:   agentBus,
		channelBus: channelBus,
		consoleBus: consoleBus,
		settings:   settings,
		pctx:       promptBuilder,
		sessions:   sessions,
		compactor:  compactor,
		tools:      registry.GetAll(),
		subagents:  subagents,
		runner:     newLoopRunner(factory.provider, settings),
		factory:    factory,
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
		case msg := <-loop.agentBus.Subscribe():
			go loop.consumeMessage(ctx, msg)
		case <-ctx.Done():
			slog.Info("Agent loop stopping")
			loop.factory.Close()
			return ctx.Err()
		}
	}
}

// ProcessDirect handles a message outside the bus (CLI, cron).
// Returns the final text response.
func (loop *AgentLoop) ProcessDirect(ctx context.Context, msg bus.AgentMessage) string {
	var res *bus.ChannelMessage
	if res = loop.routeMessage(ctx, msg); res == nil {
		return ""
	}

	return res.Content()
}

func (loop *AgentLoop) consumeMessage(ctx context.Context, msg bus.AgentMessage) {
	resp := loop.routeMessage(ctx, msg)

	if msg.Channel() == bus.ChannelCLI {
		// Route CLI responses to the console bus, not the channel bus.
		out := bus.NewChannelMessageBuilder(msg.Channel(), msg.ChatId(), "").
			Metadata(msg.Metadata()).
			Build()
		if resp != nil {
			out = *resp
		}
		loop.consoleBus.Publish(out)
	} else if resp != nil {
		loop.channelBus.Publish(*resp)
	}
}

// routeMessage dispatches msg to the appropriate channel-kind handler.
func (loop *AgentLoop) routeMessage(ctx context.Context, msg bus.AgentMessage) *bus.ChannelMessage {
	switch msg.Channel() {
	case bus.ChannelSystem:
		return loop.handleSystemChannel(ctx, msg)
	case bus.ChannelCLI:
		return loop.handleCLIChannel(ctx, msg)
	case bus.ChannelCron:
		return loop.handleCronChannel(ctx, msg)
	case bus.ChannelHeartbeat:
		return loop.handleHeartbeatChannel(ctx, msg)
	default:
		return loop.handleExternalChannel(ctx, msg)
	}
}

// handleSystemChannel processes system-channel messages injected by subagents.
// It parses the original channel/chat from msg.ChatId, runs one LLM summarisation
// turn, and routes the reply to the original chat.
func (loop *AgentLoop) handleSystemChannel(ctx context.Context, msg bus.AgentMessage) *bus.ChannelMessage {
	channelStr, chatId, _ := strings.Cut(msg.ChatId(), ":")
	if chatId == "" {
		channelStr = "cli"
		chatId = msg.ChatId()
	}
	channel := bus.Channel(channelStr)

	slog.Info("Processing system message", "sender", msg.SenderId())

	key := channelStr + ":" + chatId
	sess := loop.sessions.GetOrCreate(key)

	ctx = tools.WithTurn(ctx, tools.TurnContext{Channel: channel, ChatID: chatId})

	conversation := loop.pctx.BuildMessages(
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

	out := bus.NewChannelMessage(channel, chatId, final)
	return &out
}

// handleCLIChannel handles messages arriving on the CLI channel.
// The full pipeline is identical to external channels; the CLI-specific
// empty-outbound signal (when MessageTool fired) is handled in handleMessage.
func (loop *AgentLoop) handleCLIChannel(ctx context.Context, msg bus.AgentMessage) *bus.ChannelMessage {
	return loop.handleExternalChannel(ctx, msg)
}

// handleCronChannel handles messages arriving on the cron channel.
// Cron always uses ProcessDirect (bypassing the bus); if a message
// somehow arrives on the bus the pipeline runs but no outbound is published.
func (loop *AgentLoop) handleCronChannel(ctx context.Context, msg bus.AgentMessage) *bus.ChannelMessage {
	loop.handleExternalChannel(ctx, msg)

	return nil
}

// handleHeartbeatChannel handles messages arriving on the heartbeat channel.
// Heartbeat always uses ProcessDirect (bypassing the bus); if a message
// somehow arrives on the bus the pipeline runs but no outbound is published.
func (loop *AgentLoop) handleHeartbeatChannel(ctx context.Context, msg bus.AgentMessage) *bus.ChannelMessage {
	loop.handleExternalChannel(ctx, msg)

	return nil
}

// handleExternalChannel processes messages from external chat platforms
// (telegram, discord, slack, whatsapp, feishu, dingtalk, email, mochat, qq).
// It runs slash commands, the full LLM loop, saves the session, and returns
// an OutboundMessage — or nil if the message tool already sent the reply.
func (loop *AgentLoop) handleExternalChannel(ctx context.Context, msg bus.AgentMessage) *bus.ChannelMessage {
	slog.Info(
		"Processing message",
		"sender", msg.SenderId(),
		"channel", msg.Channel(),
		"content", llmutils.Truncate(msg.Content(), 80),
	)

	key := msg.RoutingKey()
	ses := loop.sessions.GetOrCreate(key)

	if resp := loop.handleSlashCommand(msg, ses, key); resp != nil {
		return resp
	}

	loop.compactor.Schedule(key, ses, false)

	ctx, msgSentChan := loop.withTurnContext(ctx, msg)

	conversation := loop.pctx.BuildMessages(
		ses.History(loop.settings.MemoryWindow),
		msg.Content(),
		msg.Media(),
		msg.Channel(),
		msg.ChatId(),
	)

	core := loop.factory.NewCoreAgent()
	final, toolsUsed := core.Execute(ctx, conversation, loop.progressCallback(msg))

	// If the message tool sent something, suppress the automatic reply.
	select {
	case <-msgSentChan:
		ses.AddUser(msg.Content())
		ses.AddAssistant(final, toolsUsed)
		loop.sessions.Save(ses)
		return nil
	default:
	}

	if final == "" {
		final = "I've completed processing but have no response to give."
	}

	slog.Info("Response", "channel", msg.Channel(), "sender", msg.SenderId(), "length", len(final))

	ses.AddUser(msg.Content())
	ses.AddAssistant(final, toolsUsed)
	loop.sessions.Save(ses)

	out := bus.NewChannelMessageBuilder(msg.Channel(), msg.ChatId(), final).
		Metadata(msg.Metadata()).
		Build()

	return &out
}

// handleSlashCommand checks msg.Content for a known slash command and handles
// it. Returns non-nil if the command was handled (caller should return early).
func (loop *AgentLoop) handleSlashCommand(
	msg bus.AgentMessage,
	ses *session.ChannelSessionImpl,
	key string,
) *bus.ChannelMessage {
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
func (loop *AgentLoop) handleCmdNew(msg bus.AgentMessage, sess *session.ChannelSessionImpl, key string) *bus.ChannelMessage {
	archived := sess.Messages()
	sess.Clear()
	loop.sessions.Save(sess)
	loop.sessions.Invalidate(key)

	tmp := session.NewArchivedSession(key, archived)
	loop.compactor.Schedule(key+":archive", tmp, true)

	out := bus.NewChannelMessageBuilder(msg.Channel(), msg.ChatId(), "New session started. Memory consolidation in progress.").
		Metadata(msg.Metadata()).
		Build()

	return &out
}

// handleCmdHelp returns the help text listing available slash commands.
func (loop *AgentLoop) handleCmdHelp(msg bus.AgentMessage) *bus.ChannelMessage {
	out := bus.NewChannelMessageBuilder(msg.Channel(), msg.ChatId(), "crystaldolphin commands:\n/new — Start a new conversation\n/help — Show available commands").
		Metadata(msg.Metadata()).
		Build()

	return &out
}

// withTurnContext decorates ctx with per-turn routing information and returns
// a channel that is closed when the message tool has sent a reply.
func (loop *AgentLoop) withTurnContext(ctx context.Context, msg bus.AgentMessage) (context.Context, chan struct{}) {
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

// progressCallback returns a function that pushes intermediate output to
// the outbound bus so clients can display streaming progress.
func (loop *AgentLoop) progressCallback(msg bus.AgentMessage) func(string) {
	return func(content string) {
		meta := map[string]any{"_progress": true}
		for k, v := range msg.Metadata() {
			meta[k] = v
		}

		out := bus.NewChannelMessageBuilder(msg.Channel(), msg.ChatId(), content).
			Metadata(meta).
			Build()

		if msg.Channel() == bus.ChannelCLI {
			loop.consoleBus.Publish(out)
		} else {
			loop.channelBus.Publish(out)
		}
	}
}
