package bus

import "github.com/crystaldolphin/crystaldolphin/internal/modeling/channel"

// OutboundMessage is a reply produced by the agent and routed to a chat platform.
// It is a pure value type — no I/O, no dependencies.
type OutboundMessage struct {
	channelName channel.ChannelName
	chatID      string
	content     string
	replyTo     string         // optional: original message ID to quote
	media       []string       // optional: local file paths to attach
	metadata    map[string]any // optional: channel-specific hints (thread_ts, parse_mode, …)
}

// NewOutboundMessage constructs an OutboundMessage with the required fields.
func NewOutboundMessage(ch channel.ChannelName, chatID, content string) OutboundMessage {
	return OutboundMessage{channelName: ch, chatID: chatID, content: content}
}

// WithReplyTo returns a copy with replyTo set.
func (m OutboundMessage) WithReplyTo(replyTo string) OutboundMessage {
	m.replyTo = replyTo
	return m
}

// WithMedia returns a copy with media set.
func (m OutboundMessage) WithMedia(media []string) OutboundMessage {
	m.media = media
	return m
}

// WithMetadata returns a copy with metadata set.
func (m OutboundMessage) WithMetadata(metadata map[string]any) OutboundMessage {
	m.metadata = metadata
	return m
}

// ToChannelMessage converts this outbound message to a channel.Message for delivery.
func (m OutboundMessage) ToChannelMessage() channel.Message {
	return channel.NewMessage(m.chatID, m.content).
		WithReplyTo(m.replyTo).
		WithMedia(m.media).
		WithMetadata(m.metadata)
}

func (m OutboundMessage) ChannelName() channel.ChannelName { return m.channelName }
func (m OutboundMessage) ChatID() string                   { return m.chatID }
func (m OutboundMessage) Content() string                  { return m.content }
func (m OutboundMessage) ReplyTo() string                  { return m.replyTo }
func (m OutboundMessage) Media() []string                  { return m.media }
func (m OutboundMessage) Metadata() map[string]any         { return m.metadata }
