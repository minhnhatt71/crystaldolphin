package bus

type Channel string

const (
	ChannelTelegram  Channel = "telegram"
	ChannelDiscord   Channel = "discord"
	ChannelSlack     Channel = "slack"
	ChannelWhatsApp  Channel = "whatsapp"
	ChannelFeishu    Channel = "feishu"
	ChannelDingTalk  Channel = "dingtalk"
	ChannelEmail     Channel = "email"
	ChannelMochat    Channel = "mochat"
	ChannelCLI       Channel = "cli"
	ChannelCron      Channel = "cron"
	ChannelHeartbeat Channel = "heartbeat"
	ChannelSystem    Channel = "system"
)

type ChatId string

const (
	ChatIdDirect ChatId = "direct"
)

// AgentBus carries messages from channels → agent.
// Channel adapters call PublishInbound; the agent loop reads via SubscribeInbound.
type AgentBus struct {
	ch chan AgentBusMessage
}

func NewAgentBus(bufSize int) *AgentBus {
	return &AgentBus{ch: make(chan AgentBusMessage, bufSize)}
}

// Publish delivers a message to the agent bus
func (b *AgentBus) Publish(msg AgentBusMessage) {
	b.ch <- msg
}

// Subscribe returns a receive-only view of the inbound channel.
func (b *AgentBus) Subscribe() <-chan AgentBusMessage {
	return b.ch
}
