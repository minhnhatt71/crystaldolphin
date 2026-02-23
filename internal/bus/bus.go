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

// NewMessageBus creates a MessageBus with the given buffer size on each direction.
// A buffer of 100 is a reasonable default for burst traffic.
func NewMessageBus(bufSize int) *MessageBus {
	return &MessageBus{
		Inbound:  make(chan InboundMessage, bufSize),
		Outbound: make(chan OutboundMessage, bufSize),
	}
}

// InboundSize returns the number of unconsumed inbound messages.
func (b *MessageBus) InboundSize() int { return len(b.Inbound) }

// OutboundSize returns the number of unconsumed outbound messages.
func (b *MessageBus) OutboundSize() int { return len(b.Outbound) }
