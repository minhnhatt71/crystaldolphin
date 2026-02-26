// Package bus defines the message types that flow between channels and the agent.
package bus

import "time"

// InboundMessage is a message received from a chat channel.
type InboundMessage struct {
	channel   string         // "telegram", "discord", "slack", "whatsapp", "cli", "system"
	senderID  string         // user identifier within the channel
	chatID    string         // chat / channel / DM identifier
	content   string         // message text
	timestamp time.Time      // when the message was received
	media     []string       // local file paths of downloaded attachments
	metadata  map[string]any // channel-specific extra data (message_id, username, …)
}

// NewInboundMessage creates an InboundMessage with Timestamp set to now.
// Use SetMedia and SetMetadata to attach optional fields.
func NewInboundMessage(channel, senderID, chatID, content string) InboundMessage {
	return InboundMessage{
		channel:   channel,
		senderID:  senderID,
		chatID:    chatID,
		content:   content,
		timestamp: time.Now(),
	}
}

func (m InboundMessage) Channel() string         { return m.channel }
func (m InboundMessage) SenderID() string        { return m.senderID }
func (m InboundMessage) ChatID() string          { return m.chatID }
func (m InboundMessage) Content() string         { return m.content }
func (m InboundMessage) Timestamp() time.Time    { return m.timestamp }
func (m InboundMessage) Media() []string         { return m.media }
func (m InboundMessage) Metadata() map[string]any { return m.metadata }

func (m *InboundMessage) SetMedia(media []string)        { m.media = media }
func (m *InboundMessage) SetMetadata(md map[string]any)  { m.metadata = md }

// SessionKey returns the unique key used to look up the conversation session.
// Format: "channel:chat_id" — mirrors nanobot's InboundMessage.session_key.
func (m InboundMessage) SessionKey() string {
	return m.channel + ":" + m.chatID
}

// ContentPreview returns a short snippet of the message content for logging.
func (m InboundMessage) ContentPreview() string {
	preview := m.content
	if len(preview) > 80 {
		preview = preview[:80] + "..."
	}
	return preview
}

// OutboundMessage is a response to be sent back through a channel.
type OutboundMessage struct {
	channel  string         // destination channel name
	chatID   string         // destination chat / channel / DM identifier
	content  string         // text to send
	replyTo  string         // original message ID to quote/reply to (optional)
	media    []string       // local file paths to attach (optional)
	metadata map[string]any // channel-specific hints (thread_ts, parse_mode, …)
}

// NewOutboundMessage creates an OutboundMessage with the required fields.
// Use SetReplyTo, SetMedia, and SetMetadata to attach optional fields.
func NewOutboundMessage(channel, chatID, content string) OutboundMessage {
	return OutboundMessage{
		channel: channel,
		chatID:  chatID,
		content: content,
	}
}

func (m OutboundMessage) Channel() string          { return m.channel }
func (m OutboundMessage) ChatID() string           { return m.chatID }
func (m OutboundMessage) Content() string          { return m.content }
func (m OutboundMessage) ReplyTo() string          { return m.replyTo }
func (m OutboundMessage) Media() []string          { return m.media }
func (m OutboundMessage) Metadata() map[string]any { return m.metadata }

func (m *OutboundMessage) SetReplyTo(id string)         { m.replyTo = id }
func (m *OutboundMessage) SetMedia(media []string)       { m.media = media }
func (m *OutboundMessage) SetMetadata(md map[string]any) { m.metadata = md }
