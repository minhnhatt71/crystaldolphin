package bus

import modelbus "github.com/crystaldolphin/crystaldolphin/internal/modeling/bus"

// InboundBus is a buffered channel that carries messages from chat platforms to the agent.
type InboundBus struct {
	ch chan modelbus.InboundMessage
}

// NewInboundBus creates an InboundBus with the given buffer capacity.
func NewInboundBus(cap int) *InboundBus {
	return &InboundBus{ch: make(chan modelbus.InboundMessage, cap)}
}

// Publish sends a message into the bus. Blocks if the buffer is full.
func (b *InboundBus) Publish(msg modelbus.InboundMessage) {
	b.ch <- msg
}

// Subscribe returns the read-only channel the agent consumes.
func (b *InboundBus) Subscribe() <-chan modelbus.InboundMessage {
	return b.ch
}
