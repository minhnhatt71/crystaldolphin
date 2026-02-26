// Package bus defines the message types that flow between channels and the agent.
package bus

import "time"

// InboundMessage is a message received from a chat channel.
type InboundMessage struct {
	Channel   string         // "telegram", "discord", "slack", "whatsapp", "cli", "system"
	SenderID  string         // user identifier within the channel
	ChatID    string         // chat / channel / DM identifier
	Content   string         // message text
	Timestamp time.Time      // when the message was received
	Media     []string       // local file paths of downloaded attachments
	Metadata  map[string]any // channel-specific extra data (message_id, username, …)
}

// SessionKey returns the unique key used to look up the conversation session.
// Format: "channel:chat_id" — mirrors nanobot's InboundMessage.session_key.
func (m InboundMessage) SessionKey() string {
	return m.Channel + ":" + m.ChatID
}

// ContentPreview returns a short snippet of the message content for logging.
func (m InboundMessage) ContentPreview() string {
	preview := m.Content
	if len(preview) > 80 {
		preview = preview[:80] + "..."
	}
	return preview
}

// OutboundMessage is a response to be sent back through a channel.
type OutboundMessage struct {
	Channel  string         // destination channel name
	ChatID   string         // destination chat / channel / DM identifier
	Content  string         // text to send
	ReplyTo  string         // original message ID to quote/reply to (optional)
	Media    []string       // local file paths to attach (optional)
	Metadata map[string]any // channel-specific hints (thread_ts, parse_mode, …)
}
