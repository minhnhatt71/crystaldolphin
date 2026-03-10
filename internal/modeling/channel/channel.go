package channel

import "context"

// Message is the pure value type delivered to a channel adapter.
// It carries everything needed to send a reply without depending on bus types.
type Message struct {
	chatId   string
	content  string
	replyTo  string         // optional: original message ID to quote
	media    []string       // optional: local file paths to attach
	metadata map[string]any // optional: channel-specific hints (thread_ts, parse_mode, …)
}

// NewMessage constructs a Message with the required fields.
func NewMessage(chatID, content string) Message {
	return Message{chatId: chatID, content: content}
}

// WithReplyTo returns a copy of the message with replyTo set.
func (m Message) WithReplyTo(replyTo string) Message {
	m.replyTo = replyTo
	return m
}

// WithMedia returns a copy of the message with media set.
func (m Message) WithMedia(media []string) Message {
	m.media = media
	return m
}

// WithMetadata returns a copy of the message with metadata set.
func (m Message) WithMetadata(metadata map[string]any) Message {
	m.metadata = metadata
	return m
}

func (m Message) ChatId() string           { return m.chatId }
func (m Message) Content() string          { return m.content }
func (m Message) ReplyTo() string          { return m.replyTo }
func (m Message) Media() []string          { return m.media }
func (m Message) Metadata() map[string]any { return m.metadata }

// Channel is the domain interface every chat-platform adapter must implement.
// Concrete implementations live in internal/channels/.
//
//   - Name identifies the adapter (e.g. "telegram", "discord", "cli").
//   - Start blocks, listening for inbound messages, until ctx is cancelled.
//   - Send delivers an outbound message to the platform.
type Channel interface {
	Name() ChannelName
	Start(ctx context.Context) error
	Send(ctx context.Context, msg Message) error
}
