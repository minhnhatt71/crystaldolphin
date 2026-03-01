package bus

// ChannelMessage is a response to be sent back through a channel.
type ChannelMessage struct {
	channel  Channel        // destination channel name
	chatId   string         // destination chat / channel / DM identifier
	content  string         // text to send
	replyTo  string         // original message ID to quote/reply to (optional)
	media    []string       // local file paths to attach (optional)
	metadata map[string]any // channel-specific hints (thread_ts, parse_mode, …)
}

func (m ChannelMessage) Channel() Channel         { return m.channel }
func (m ChannelMessage) ChatId() string           { return m.chatId }
func (m ChannelMessage) Content() string          { return m.content }
func (m ChannelMessage) ReplyTo() string          { return m.replyTo }
func (m ChannelMessage) Media() []string          { return m.media }
func (m ChannelMessage) Metadata() map[string]any { return m.metadata }

func NewChannelMessage(channel Channel, chatId, content string) ChannelMessage {
	return ChannelMessage{
		channel: channel,
		chatId:  chatId,
		content: content,
	}
}

type ChannelMessageBuilder struct {
	channel  Channel
	chatId   string
	content  string
	media    []string
	metadata map[string]any
}

func NewChannelMessageBuilder(channel Channel, chatId, content string) *ChannelMessageBuilder {
	return &ChannelMessageBuilder{
		channel: channel,
		chatId:  chatId,
		content: content,
	}
}

func (b *ChannelMessageBuilder) Media(media []string) *ChannelMessageBuilder {
	b.media = media
	return b
}

func (b *ChannelMessageBuilder) Metadata(md map[string]any) *ChannelMessageBuilder {
	b.metadata = md
	return b
}

func (b *ChannelMessageBuilder) Build() ChannelMessage {
	return ChannelMessage{
		channel:  b.channel,
		chatId:   b.chatId,
		content:  b.content,
		media:    b.media,
		metadata: b.metadata,
	}
}
