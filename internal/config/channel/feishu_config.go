package channel

// FeishuConfig configures the Feishu/Lark channel.
type FeishuConfig struct {
	Enabled           bool     `json:"enabled"`
	AppID             string   `json:"appId"`
	AppSecret         string   `json:"appSecret"`
	EncryptKey        string   `json:"encryptKey"`
	VerificationToken string   `json:"verificationToken"`
	AllowFrom         []string `json:"allowFrom"`
}

func DefaultFeishuConfig() FeishuConfig {
	return FeishuConfig{AllowFrom: []string{}}
}
