package agentlegacy

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"strings"

	"github.com/crystaldolphin/crystaldolphin/internal/buslegacy"
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
	settings schema.AgentSettings

	agentBus   *buslegacy.AgentBus
	channelBus *buslegacy.ChannelBus
	prompt     *PromptContext
	sessions   *session.Manager
	compactor  schema.MemoryCompactor
	tools      tools.ToolList // MCP registration target; factory holds &loop.tools
	subagents  *SubagentManager
	runner     LoopRunner    // shared LLM iteration logic (used by handleSystemChannel)
	factory    *AgentFactory // creates per-request CoreAgent / SubAgent instances
}

// NewAgentLoop creates an AgentLoop with the supplied factory, tool registry, and
// subagent manager.
func NewAgentLoop(
	agentBus *buslegacy.AgentBus,
	channelBus *buslegacy.ChannelBus,
	factory *AgentFactory,
	settings schema.AgentSettings,
	sessions *session.Manager,
	compactor schema.MemoryCompactor,
	registry *tools.Registry,
	subAgentManager *SubagentManager,
	promptBuilder *PromptContext,
) *AgentLoop {
	loop := &AgentLoop{
		agentBus:   agentBus,
		channelBus: channelBus,
		settings:   settings,
		prompt:     promptBuilder,
		sessions:   sessions,
		compactor:  compactor,
		tools:      registry.GetAll(),
		subagents:  subAgentManager,
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
			go loop.process(ctx, msg)
		case <-ctx.Done():
			slog.Info("Agent loop stopping")
			loop.factory.Close()
			return ctx.Err()
		}
	}
}

// ProcessDirect handles a message outside the bus (CLI, cron).
// Returns the final text response.
func (loop *AgentLoop) ProcessDirect(ctx context.Context, msg buslegacy.AgentMessage) string {
	var res *buslegacy.ChannelMessage
	if res = loop.routeMessage(ctx, msg); res == nil {
		return ""
	}

	return res.Content()
}

func (loop *AgentLoop) process(ctx context.Context, msg buslegacy.AgentMessage) {
	resp := loop.routeMessage(ctx, msg)

	if msg.Channel() == buslegacy.ChannelCLI {
		out := buslegacy.NewChannelMessageBuilder(msg.Channel(), msg.ChatId(), "").
			Metadata(msg.Metadata()).
			Build()

		if resp != nil {
			out = *resp
		}

		loop.channelBus.Publish(out)
	} else if resp != nil {
		loop.channelBus.Publish(*resp)
	}
}

// routeMessage dispatches msg to the appropriate channel-kind handler.
func (loop *AgentLoop) routeMessage(ctx context.Context, msg buslegacy.AgentMessage) *buslegacy.ChannelMessage {
	switch msg.Channel() {
	case buslegacy.ChannelSystem:
		return loop.handleSystemChannel(ctx, msg)
	case buslegacy.ChannelCLI:
		return loop.handleCLIChannel(ctx, msg)
	case buslegacy.ChannelCron:
		return loop.handleCronChannel(ctx, msg)
	case buslegacy.ChannelHeartbeat:
		return loop.handleHeartbeatChannel(ctx, msg)
	default:
		return loop.consumeMessage(ctx, msg)
	}
}

// handleSystemChannel processes system-channel messages injected by subagents.
// It parses the original channel/chat from msg.ChatId, runs one LLM summarisation
// turn, and routes the reply to the original chat.
func (loop *AgentLoop) handleSystemChannel(ctx context.Context, msg buslegacy.AgentMessage) *buslegacy.ChannelMessage {
	channelStr, chatId, _ := strings.Cut(msg.ChatId(), ":")
	if chatId == "" {
		channelStr = "cli"
		chatId = msg.ChatId()
	}

	channel := buslegacy.Channel(channelStr)

	slog.Info("Processing system message", "sender", msg.SenderId())

	sess := loop.sessions.GetOrCreate(buslegacy.RoutingKey(channel, chatId))

	ctx = tools.WithTurn(ctx, channel, chatId, "")

	conversation := loop.prompt.BuildMessages(
		sess.History(loop.settings.MemoryWindow),
		msg.Content(),
		nil,
		channel,
		chatId,
	)

	final, _ := loop.runner.run(ctx, conversation, &loop.tools, nil)
	result := llmutils.StringOrDefault(final, "Background task completed.")

	sess.
		RecordUserMessage(fmt.Sprintf("[System: %s] %s", msg.SenderId(), msg.Content())).
		RecordAssistantMessage(result, nil)

	loop.sessions.Save(sess)

	out := buslegacy.NewChannelMessage(channel, chatId, result)
	return &out
}

// handleCLIChannel handles messages arriving on the CLI channel.
// The full pipeline is identical to external channels; the CLI-specific
// empty-outbound signal (when MessageTool fired) is handled in handleMessage.
func (loop *AgentLoop) handleCLIChannel(ctx context.Context, msg buslegacy.AgentMessage) *buslegacy.ChannelMessage {
	return loop.consumeMessage(ctx, msg)
}

// handleCronChannel handles messages arriving on the cron channel.
// Cron always uses ProcessDirect (bypassing the bus); if a message
// somehow arrives on the bus the pipeline runs but no outbound is published.
func (loop *AgentLoop) handleCronChannel(ctx context.Context, msg buslegacy.AgentMessage) *buslegacy.ChannelMessage {
	loop.consumeMessage(ctx, msg)

	return nil
}

// handleHeartbeatChannel handles messages arriving on the heartbeat channel.
// Heartbeat always uses ProcessDirect (bypassing the bus); if a message
// somehow arrives on the bus the pipeline runs but no outbound is published.
func (loop *AgentLoop) handleHeartbeatChannel(ctx context.Context, msg buslegacy.AgentMessage) *buslegacy.ChannelMessage {
	loop.consumeMessage(ctx, msg)

	return nil
}

// consumeMessage processes messages from external chat platforms
// (telegram, discord, slack, whatsapp, feishu, dingtalk, email, mochat, qq).
// It runs slash commands, the full LLM loop, saves the session, and returns
// an OutboundMessage — or nil if the message tool already sent the reply.
func (loop *AgentLoop) consumeMessage(ctx context.Context, msg buslegacy.AgentMessage) *buslegacy.ChannelMessage {
	slog.Info(
		"Processing message",
		"sender", msg.SenderId(),
		"channel", msg.Channel(),
		"content", llmutils.Truncate(msg.Content(), 80),
	)

	key := msg.RoutingKey()
	sess := loop.sessions.GetOrCreate(key)

	if resp := loop.handleSlashCommand(msg, sess, key); resp != nil {
		return resp
	}

	loop.compactor.Schedule(key, sess, false)

	ctx = createTurnContext(ctx, msg)

	conversation := loop.prompt.BuildMessages(
		sess.History(loop.settings.MemoryWindow),
		msg.Content(),
		msg.Media(),
		msg.Channel(),
		msg.ChatId(),
	)

	primaryAgent := loop.factory.NewPrimaryAgent()
	final, toolsUsed := primaryAgent.Execute(ctx, conversation, loop.progressCallback(msg))
	result := llmutils.StringOrDefault(final, "Sorry, I couldn't generate a response.")

	loop.sessions.Save(
		sess.
			RecordUserMessage(msg.Content()).
			RecordAssistantMessage(result, toolsUsed),
	)

	if tools.TurnCtx(ctx).PublishedToChannels() {
		slog.Info("Message tool already sent a reply, suppressing automatic response")
		return nil
	}

	slog.Info("Response",
		"channel", msg.Channel(),
		"sender", msg.SenderId(),
		"length", len(result),
	)

	out := buslegacy.NewChannelMessageBuilder(msg.Channel(), msg.ChatId(), result).
		Metadata(msg.Metadata()).
		Build()

	return &out
}

// handleSlashCommand checks msg.Content for a known slash command and handles
// it. Returns non-nil if the command was handled (caller should return early).
func (loop *AgentLoop) handleSlashCommand(
	msg buslegacy.AgentMessage,
	ses *session.ChannelSessionImpl,
	key string,
) *buslegacy.ChannelMessage {
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
func (loop *AgentLoop) handleCmdNew(msg buslegacy.AgentMessage, sess *session.ChannelSessionImpl, key string) *buslegacy.ChannelMessage {
	archived := sess.Messages()
	sess.Clear()
	loop.sessions.Save(sess)
	loop.sessions.Invalidate(key)

	tmp := session.NewArchivedSession(key, archived)
	loop.compactor.Schedule(key+":archive", tmp, true)

	out := buslegacy.NewChannelMessageBuilder(msg.Channel(), msg.ChatId(), "New session started. Memory consolidation in progress.").
		Metadata(msg.Metadata()).
		Build()

	return &out
}

// handleCmdHelp returns the help text listing available slash commands.
func (loop *AgentLoop) handleCmdHelp(msg buslegacy.AgentMessage) *buslegacy.ChannelMessage {
	out := buslegacy.NewChannelMessageBuilder(
		msg.Channel(),
		msg.ChatId(),
		"crystaldolphin commands:\n/new — Start a new conversation\n/help — Show available commands",
	).
		Metadata(msg.Metadata()).
		Build()

	return &out
}

// createTurnContext decorates ctx with per-turn routing information and returns
// a flag that is set to true when the message tool has sent a reply.
func createTurnContext(ctx context.Context, msg buslegacy.AgentMessage) context.Context {
	msgId := ""
	if v, ok := msg.Metadata()["message_id"].(string); ok {
		msgId = v
	}

	return tools.WithTurn(ctx, msg.Channel(), msg.ChatId(), msgId)
}

// progressCallback returns a function that pushes intermediate output to
// the outbound bus so clients can display streaming progress.
func (loop *AgentLoop) progressCallback(msg buslegacy.AgentMessage) func(string) {
	return func(content string) {
		meta := map[string]any{"_progress": true}
		maps.Copy(meta, msg.Metadata())

		out := buslegacy.NewChannelMessageBuilder(msg.Channel(), msg.ChatId(), content).
			Metadata(meta).
			Build()

		loop.channelBus.Publish(out)
	}
}
