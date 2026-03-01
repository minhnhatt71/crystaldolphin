package bus

// ChannelBus carries messages from agent → channels.
// The agent loop calls PublishOutbound; the channel manager reads via SubscribeOutbound.
type ChannelBus struct {
	ch chan ChannelMessage
}

func NewChannelBus(bufSize int) *ChannelBus {
	return &ChannelBus{ch: make(chan ChannelMessage, bufSize)}
}

// Publish delivers a response from the agent to the channel manager.
func (b *ChannelBus) Publish(msg ChannelMessage) {
	b.ch <- msg
}

// Subscribe returns a receive-only view of the outbound channel.
func (b *ChannelBus) Subscribe() <-chan ChannelMessage {
	return b.ch
}
