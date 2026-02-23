package channel

type DingTalkConfig struct {
	Enabled      bool     `json:"enabled"`
	ClientID     string   `json:"clientId"`
	ClientSecret string   `json:"clientSecret"`
	AllowFrom    []string `json:"allowFrom"`
}

func DefaultDingTalkConfig() DingTalkConfig {
	return DingTalkConfig{AllowFrom: []string{}}
}
