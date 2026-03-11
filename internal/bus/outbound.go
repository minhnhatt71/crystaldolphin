package bus

import modelbus "github.com/crystaldolphin/crystaldolphin/internal/modeling/bus"

// OutboundBus is a buffered channel that carries agent replies to the channel manager.
type OutboundBus struct {
	ch chan modelbus.OutboundMessage
}

// NewOutboundBus creates an OutboundBus with the given buffer capacity.
func NewOutboundBus(cap int) *OutboundBus {
	return &OutboundBus{ch: make(chan modelbus.OutboundMessage, cap)}
}

// Publish sends a message into the bus. Blocks if the buffer is full.
func (b *OutboundBus) Publish(msg modelbus.OutboundMessage) {
	b.ch <- msg
}

// Subscribe returns the read-only channel the manager consumes.
func (b *OutboundBus) Subscribe() <-chan modelbus.OutboundMessage {
	return b.ch
}
