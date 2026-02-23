package channel

type ChannelsConfig struct {
	WhatsApp WhatsAppConfig `json:"whatsapp"`
	Telegram TelegramConfig `json:"telegram"`
	Discord  DiscordConfig  `json:"discord"`
	Feishu   FeishuConfig   `json:"feishu"`
	Mochat   MochatConfig   `json:"mochat"`
	DingTalk DingTalkConfig `json:"dingtalk"`
	Email    EmailConfig    `json:"email"`
	Slack    SlackConfig    `json:"slack"`
	QQ       QQConfig       `json:"qq"`
}

func DefaultChannelsConfig() ChannelsConfig {
	return ChannelsConfig{
		WhatsApp: DefaultWhatsAppConfig(),
		Telegram: DefaultTelegramConfig(),
		Discord:  DefaultDiscordConfig(),
		Feishu:   DefaultFeishuConfig(),
		Mochat:   DefaultMochatConfig(),
		DingTalk: DefaultDingTalkConfig(),
		Email:    DefaultEmailConfig(),
		Slack:    DefaultSlackConfig(),
		QQ:       DefaultQQConfig(),
	}
}
