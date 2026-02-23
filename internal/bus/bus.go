package bus

// MessageBus decouples chat channels from the agent core.
//
// Channels push InboundMessages; the agent consumes them, processes, and
// pushes OutboundMessages back for the channel manager to route.
// Both directions use buffered channels so senders never block on a slow consumer.
type MessageBus struct {
	Inbound  chan InboundMessage  // channels → agent
	Outbound chan OutboundMessage // agent → channels
}

func NewMessageBus(bufSize int) *MessageBus {
	return &MessageBus{
		Inbound:  make(chan InboundMessage, bufSize),
		Outbound: make(chan OutboundMessage, bufSize),
	}
}

func (b *MessageBus) InboundSize() int { return len(b.Inbound) }

func (b *MessageBus) OutboundSize() int { return len(b.Outbound) }
