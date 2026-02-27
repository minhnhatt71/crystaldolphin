package bus

// OutboundMessage is a response to be sent back through a channel.
type OutboundMessage struct {
	channel  string         // destination channel name
	chatId   string         // destination chat / channel / DM identifier
	content  string         // text to send
	replyTo  string         // original message ID to quote/reply to (optional)
	media    []string       // local file paths to attach (optional)
	metadata map[string]any // channel-specific hints (thread_ts, parse_mode, …)
}

func (m OutboundMessage) Channel() string                { return m.channel }
func (m OutboundMessage) ChatId() string                 { return m.chatId }
func (m OutboundMessage) Content() string                { return m.content }
func (m OutboundMessage) ReplyTo() string                { return m.replyTo }
func (m OutboundMessage) Media() []string                { return m.media }
func (m OutboundMessage) Metadata() map[string]any       { return m.metadata }
func (m *OutboundMessage) SetMedia(media []string)       { m.media = media }
func (m *OutboundMessage) SetMetadata(md map[string]any) { m.metadata = md }

func NewOutboundMessage(channel, chatId, content string) OutboundMessage {
	return OutboundMessage{
		channel: channel,
		chatId:  chatId,
		content: content,
	}
}
