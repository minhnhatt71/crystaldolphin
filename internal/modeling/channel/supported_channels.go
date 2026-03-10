package channel

type ChannelName string

const (
	ChannelTelegram  ChannelName = "telegram"
	ChannelDiscord   ChannelName = "discord"
	ChannelSlack     ChannelName = "slack"
	ChannelWhatsApp  ChannelName = "whatsapp"
	ChannelFeishu    ChannelName = "feishu"
	ChannelDingTalk  ChannelName = "dingtalk"
	ChannelEmail     ChannelName = "email"
	ChannelMochat    ChannelName = "mochat"
	ChannelCLI       ChannelName = "cli"
	ChannelCron      ChannelName = "cron"
	ChannelHeartbeat ChannelName = "heartbeat"
	ChannelSystem    ChannelName = "system"
)
