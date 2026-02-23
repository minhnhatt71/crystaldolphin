package channel

// TelegramConfig configures the Telegram channel.
type TelegramConfig struct {
	Enabled        bool     `json:"enabled"`
	Token          string   `json:"token"`
	AllowFrom      []string `json:"allowFrom"`
	Proxy          string   `json:"proxy,omitempty"`
	ReplyToMessage bool     `json:"replyToMessage"`
}

func DefaultTelegramConfig() TelegramConfig {
	return TelegramConfig{AllowFrom: []string{}}
}
