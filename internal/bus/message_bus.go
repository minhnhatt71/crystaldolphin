package bus

type ChannelType string

const (
	ChannelTelegram  ChannelType = "telegram"
	ChannelDiscord   ChannelType = "discord"
	ChannelSlack     ChannelType = "slack"
	ChannelWhatsApp  ChannelType = "whatsapp"
	ChannelFeishu    ChannelType = "feishu"
	ChannelDingTalk  ChannelType = "dingtalk"
	ChannelEmail     ChannelType = "email"
	ChannelMochat    ChannelType = "mochat"
	ChannelCLI       ChannelType = "cli"
	ChannelCron      ChannelType = "cron"
	ChannelHeartbeat ChannelType = "heartbeat"
	ChannelSystem    ChannelType = "system"
)

type ChatId string

const (
	ChatIdDirect ChatId = "direct"
)

// Bus is the contract between chat channels and the agent core.
// Implementations may use buffered channels, pub/sub systems, or any other transport.
type Bus interface {
	// PublishInbound delivers a message from a channel to the agent.
	PublishInbound(msg InboundMessage)
	// PublishOutbound delivers a response from the agent to a channel.
	PublishOutbound(msg OutboundMessage)
	// InboundChan returns a receive-only channel for the agent to consume.
	InboundChan() <-chan InboundMessage
	// OutboundChan returns a receive-only channel for the channel manager to consume.
	OutboundChan() <-chan OutboundMessage
}

// MessageBus is the default in-process Bus implementation backed by buffered Go channels.
//
// Channels push InboundMessages; the agent consumes them, processes, and
// pushes OutboundMessages back for the channel manager to route.
// Both directions use buffered channels so senders never block on a slow consumer.
type MessageBus struct {
	inbound  chan InboundMessage  // channels -> backend
	outbound chan OutboundMessage // backend -> channels
}

func NewMessageBus(bufSize int) Bus {
	return &MessageBus{
		inbound:  make(chan InboundMessage, bufSize),
		outbound: make(chan OutboundMessage, bufSize),
	}
}

// PublishInbound sends an InboundMessage to the agent.
func (b *MessageBus) PublishInbound(msg InboundMessage) {
	b.inbound <- msg
}

// PublishOutbound sends an OutboundMessage to the channel manager.
func (b *MessageBus) PublishOutbound(msg OutboundMessage) {
	b.outbound <- msg
}

// InboundChan returns a receive-only view of the inbound channel.
func (b *MessageBus) InboundChan() <-chan InboundMessage {
	return b.inbound
}

// OutboundChan returns a receive-only view of the outbound channel.
func (b *MessageBus) OutboundChan() <-chan OutboundMessage {
	return b.outbound
}
