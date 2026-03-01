package bus

// AgentBus contains messages from channels to be consumed by agents
type AgentBus struct {
	channel chan AgentMessage
}

func NewAgentBus(bufSize int) *AgentBus {
	return &AgentBus{channel: make(chan AgentMessage, bufSize)}
}

// Publish delivers a message to the agent bus
func (b *AgentBus) Publish(msg AgentMessage) {
	b.channel <- msg
}

// Subscribe returns a receive-only view of the inbound channel.
func (b *AgentBus) Subscribe() <-chan AgentMessage {
	return b.channel
}
