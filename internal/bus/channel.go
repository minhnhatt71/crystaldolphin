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
