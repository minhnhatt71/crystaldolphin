// Package bus defines the message types that flow between channels and the agent.
package bus

import "time"

type SenderId string

const SenderIdCLI string = "user"
const SenderIdSubAgent string = "subagent"

// AgentBusMessage is a message received from a chat channel.
type AgentBusMessage struct {
	channel    Channel        // "telegram", "discord", "slack", "whatsapp", "cli", "system"
	chatId     string         // chat / channel / DM identifier
	senderId   string         // user identifier within the channel
	routingKey string         // optional override; empty means derive from channel:chatId
	content    string         // message text
	timestamp  time.Time      // when the message was received
	media      []string       // local file paths of downloaded attachments
	metadata   map[string]any // channel-specific extra data (message_id, username, …)
}

// NewAgentBusMessage creates an InboundMessage with Timestamp set to now.
// routingKey overrides the default "channel:chatId" session key; pass "" to use the default.
// Use SetMedia and SetMetadata to attach optional fields.
func NewAgentBusMessage(channel Channel, senderId, chatId, content, routingKey string) AgentBusMessage {
	return AgentBusMessage{
		channel:    channel,
		senderId:   senderId,
		chatId:     chatId,
		content:    content,
		routingKey: routingKey,
		timestamp:  time.Now(),
	}
}

func (m AgentBusMessage) ChatId() string                 { return m.chatId }
func (m AgentBusMessage) SenderId() string               { return m.senderId }
func (m AgentBusMessage) Content() string                { return m.content }
func (m AgentBusMessage) Channel() Channel               { return m.channel }
func (m AgentBusMessage) Timestamp() time.Time           { return m.timestamp }
func (m AgentBusMessage) Media() []string                { return m.media }
func (m AgentBusMessage) Metadata() map[string]any       { return m.metadata }
func (m *AgentBusMessage) SetMedia(media []string)       { m.media = media }
func (m *AgentBusMessage) SetMetadata(md map[string]any) { m.metadata = md }

// RoutingKey returns the unique key used to look up the conversation session.
// If an explicit key was set via SetRoutingKey, it is returned;
// otherwise falls back to "channel:chat_id" — mirrors nanobot's InboundMessage.session_key.
func (m AgentBusMessage) RoutingKey() string {
	if m.routingKey != "" {
		return m.routingKey
	}

	return string(m.channel) + ":" + m.chatId
}
