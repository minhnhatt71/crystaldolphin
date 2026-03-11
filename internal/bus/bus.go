package bus

import modelbus "github.com/crystaldolphin/crystaldolphin/internal/modeling/bus"

// Compile-time assertion: MessageBus satisfies the domain Bus interface.
var _ modelbus.Bus = (*MessageBus)(nil)

// MessageBus wires together an InboundBus and an OutboundBus, implementing
// the domain modeling/bus.Bus contract.
//
// Use New to create one; pass it to both the agent and the channel manager.
type MessageBus struct {
	inbound  *InboundBus
	outbound *OutboundBus
}

// New creates a MessageBus with buffered inbound and outbound queues.
// cap is applied to both queues.
func New(cap int) *MessageBus {
	return &MessageBus{
		inbound:  NewInboundBus(cap),
		outbound: NewOutboundBus(cap),
	}
}

// PublishInbound delivers an inbound message from a channel to the agent.
func (b *MessageBus) PublishInbound(msg modelbus.InboundMessage) {
	b.inbound.Publish(msg)
}

// SubscribeInbound returns the read-only channel the agent reads.
func (b *MessageBus) SubscribeInbound() <-chan modelbus.InboundMessage {
	return b.inbound.Subscribe()
}

// PublishOutbound delivers an outbound reply from the agent to the channel manager.
func (b *MessageBus) PublishOutbound(msg modelbus.OutboundMessage) {
	b.outbound.Publish(msg)
}

// SubscribeOutbound returns the read-only channel the manager reads.
func (b *MessageBus) SubscribeOutbound() <-chan modelbus.OutboundMessage {
	return b.outbound.Subscribe()
}

// Inbound returns the underlying InboundBus for direct use.
func (b *MessageBus) Inbound() *InboundBus { return b.inbound }

// Outbound returns the underlying OutboundBus for direct use.
func (b *MessageBus) Outbound() *OutboundBus { return b.outbound }
