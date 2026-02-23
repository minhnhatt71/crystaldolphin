package channel

// QQConfig configures the QQ channel.
type QQConfig struct {
	Enabled   bool     `json:"enabled"`
	AppID     string   `json:"appId"`
	Secret    string   `json:"secret"`
	AllowFrom []string `json:"allowFrom"`
}

func DefaultQQConfig() QQConfig {
	return QQConfig{AllowFrom: []string{}}
}
