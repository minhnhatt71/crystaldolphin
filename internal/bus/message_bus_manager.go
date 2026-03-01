package bus

// MessageBusManager groups the three message buses used across the system:
//   - Agent: channels → agent (inbound)
//   - Channel: agent → external chat channels (outbound)
//   - Console: agent → CLI REPL (separate so CLI output is not drained by the channel manager)
type MessageBusManager struct {
	agent   *AgentBus
	channel *ChannelBus
	console *ConsoleBus
}

func NewMessageBusManager(bufSize int) *MessageBusManager {
	return &MessageBusManager{
		agent:   NewAgentBus(bufSize),
		channel: NewChannelBus(bufSize),
		console: NewConsoleBus(bufSize),
	}
}

func (m *MessageBusManager) AgentBus() *AgentBus     { return m.agent }
func (m *MessageBusManager) ChannelBus() *ChannelBus { return m.channel }
func (m *MessageBusManager) ConsoleBus() *ConsoleBus { return m.console }
