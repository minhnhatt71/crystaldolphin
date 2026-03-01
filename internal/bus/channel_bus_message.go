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

func (m ChannelMessage) Channel() Channel               { return m.channel }
func (m ChannelMessage) ChatId() string                 { return m.chatId }
func (m ChannelMessage) Content() string                { return m.content }
func (m ChannelMessage) ReplyTo() string                { return m.replyTo }
func (m ChannelMessage) Media() []string                { return m.media }
func (m ChannelMessage) Metadata() map[string]any       { return m.metadata }
func (m *ChannelMessage) SetMedia(media []string)       { m.media = media }
func (m *ChannelMessage) SetMetadata(md map[string]any) { m.metadata = md }

func NewChannelMessage(channel Channel, chatId, content string) ChannelMessage {
	return ChannelMessage{
		channel: channel,
		chatId:  chatId,
		content: content,
	}
}
