// Package bus defines the message types that flow between channels and the agent.
package bus

import "time"

type SenderId string

const SenderIdCLI string = "user"
const SenderIdSubAgent string = "subagent"

// AgentMessage is a message received from a chat channel.
type AgentMessage struct {
	channel    Channel        // "telegram", "discord", "slack", "whatsapp", "cli", "system"
	chatId     string         // chat / channel / DM identifier
	senderId   string         // user identifier within the channel
	routingKey string         // optional override; empty means derive from channel:chatId
	content    string         // message text
	timestamp  time.Time      // when the message was received
	media      []string       // local file paths of downloaded attachments
	metadata   map[string]any // channel-specific extra data (message_id, username, …)
}

// NewAgentMessage creates an InboundMessage with Timestamp set to now.
// routingKey overrides the default "channel:chatId" session key; pass "" to use the default.
// Use SetMedia and SetMetadata to attach optional fields.
func NewAgentMessage(channel Channel, senderId, chatId, content, routingKey string) AgentMessage {
	key := routingKey
	if key == "" {
		key = RoutingKey(channel, chatId)
	}

	return AgentMessage{
		channel:    channel,
		senderId:   senderId,
		chatId:     chatId,
		content:    content,
		routingKey: key,
		timestamp:  time.Now(),
	}
}

func (m AgentMessage) ChatId() string           { return m.chatId }
func (m AgentMessage) SenderId() string         { return m.senderId }
func (m AgentMessage) Content() string          { return m.content }
func (m AgentMessage) Channel() Channel         { return m.channel }
func (m AgentMessage) Timestamp() time.Time     { return m.timestamp }
func (m AgentMessage) Media() []string          { return m.media }
func (m AgentMessage) Metadata() map[string]any { return m.metadata }

// RoutingKey returns the unique key used to look up the conversation session.
// If an explicit key was set via SetRoutingKey, it is returned;
// otherwise falls back to "channel:chat_id" — mirrors nanobot's InboundMessage.session_key.
func (m AgentMessage) RoutingKey() string {
	return m.routingKey
}

type AgentMessageBuilder struct {
	channel    Channel
	senderId   string
	chatId     string
	content    string
	routingKey string
	media      []string
	metadata   map[string]any
}

func NewAgentMessageBuilder(channel Channel, senderId, chatId, content string) *AgentMessageBuilder {
	return &AgentMessageBuilder{
		channel:  channel,
		senderId: senderId,
		chatId:   chatId,
		content:  content,
	}
}

func (b *AgentMessageBuilder) Media(media []string) *AgentMessageBuilder {
	b.media = media
	return b
}

func (b *AgentMessageBuilder) Metadata(md map[string]any) *AgentMessageBuilder {
	b.metadata = md
	return b
}

func (b *AgentMessageBuilder) Build() AgentMessage {
	key := b.routingKey
	if key == "" {
		key = RoutingKey(b.channel, b.chatId)
	}

	return AgentMessage{
		channel:    b.channel,
		senderId:   b.senderId,
		chatId:     b.chatId,
		content:    b.content,
		routingKey: key,
		timestamp:  time.Now(),
		media:      b.media,
		metadata:   b.metadata,
	}
}
