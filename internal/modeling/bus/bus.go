package bus

// Bus is the domain contract for the message bus that connects chat-platform
// channels to the agent.
//
//   - PublishInbound delivers a message arriving from a channel to the agent.
//   - SubscribeInbound returns a channel the agent reads to receive messages.
//   - PublishOutbound delivers a response from the agent to the channel manager.
//   - SubscribeOutbound returns a channel the manager reads to route replies.
type Bus interface {
	PublishInbound(msg InboundMessage)
	SubscribeInbound() <-chan InboundMessage

	PublishOutbound(msg OutboundMessage)
	SubscribeOutbound() <-chan OutboundMessage
}
