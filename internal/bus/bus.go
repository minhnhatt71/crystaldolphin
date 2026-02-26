package bus

// Bus is the contract between chat channels and the agent core.
// Implementations may use buffered channels, pub/sub systems, or any other transport.
type Bus interface {
	// PublishInbound delivers a message from a channel to the agent.
	PublishInbound(msg InboundMessage)
	// PublishOutbound delivers a response from the agent to a channel.
	PublishOutbound(msg OutboundMessage)
	// SubscribeInbound returns a receive-only channel for the agent to consume.
	SubscribeInbound() <-chan InboundMessage
	// SubscribeOutbound returns a receive-only channel for the channel manager to consume.
	SubscribeOutbound() <-chan OutboundMessage
}

// MessageBus is the default in-process Bus implementation backed by buffered Go channels.
//
// Channels push InboundMessages; the agent consumes them, processes, and
// pushes OutboundMessages back for the channel manager to route.
// Both directions use buffered channels so senders never block on a slow consumer.
type MessageBus struct {
	inbound  chan InboundMessage  // channels -> agent
	outbound chan OutboundMessage // agent -> channels
}

func NewMessageBus(bufSize int) *MessageBus {
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

// SubscribeInbound returns a receive-only view of the inbound channel.
func (b *MessageBus) SubscribeInbound() <-chan InboundMessage {
	return b.inbound
}

// SubscribeOutbound returns a receive-only view of the outbound channel.
func (b *MessageBus) SubscribeOutbound() <-chan OutboundMessage {
	return b.outbound
}
