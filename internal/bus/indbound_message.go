// Package bus defines the message types that flow between channels and the agent.
package bus

import "time"

// InboundMessage is a message received from a chat channel.
type InboundMessage struct {
	channel   string         // "telegram", "discord", "slack", "whatsapp", "cli", "system"
	senderId  string         // user identifier within the channel
	chatId    string         // chat / channel / DM identifier
	content   string         // message text
	timestamp time.Time      // when the message was received
	media     []string       // local file paths of downloaded attachments
	metadata  map[string]any // channel-specific extra data (message_id, username, …)
}

// NewInboundMessage creates an InboundMessage with Timestamp set to now.
// Use SetMedia and SetMetadata to attach optional fields.
func NewInboundMessage(channel, senderId, chatId, content string) InboundMessage {
	return InboundMessage{
		channel:   channel,
		senderId:  senderId,
		chatId:    chatId,
		content:   content,
		timestamp: time.Now(),
	}
}

func (m InboundMessage) ChatId() string                 { return m.chatId }
func (m InboundMessage) SenderId() string               { return m.senderId }
func (m InboundMessage) Content() string                { return m.content }
func (m InboundMessage) Channel() string                { return m.channel }
func (m InboundMessage) Timestamp() time.Time           { return m.timestamp }
func (m InboundMessage) Media() []string                { return m.media }
func (m InboundMessage) Metadata() map[string]any       { return m.metadata }
func (m *InboundMessage) SetMedia(media []string)       { m.media = media }
func (m *InboundMessage) SetMetadata(md map[string]any) { m.metadata = md }

// SessionKey returns the unique key used to look up the conversation session.
// Format: "channel:chat_id" — mirrors nanobot's InboundMessage.session_key.
func (m InboundMessage) SessionKey() string {
	return m.channel + ":" + m.chatId
}

// Preview returns a short snippet of the message content for logging.
func (m InboundMessage) Preview() string {
	preview := m.content
	if len(preview) > 80 {
		preview = preview[:80] + "..."
	}
	return preview
}
