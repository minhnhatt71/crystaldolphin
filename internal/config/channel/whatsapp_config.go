package channel

// WhatsAppConfig configures the WhatsApp channel.
type WhatsAppConfig struct {
	Enabled     bool     `json:"enabled"`
	BridgeURL   string   `json:"bridgeUrl"`
	BridgeToken string   `json:"bridgeToken"`
	AllowFrom   []string `json:"allowFrom"`
}

func DefaultWhatsAppConfig() WhatsAppConfig {
	return WhatsAppConfig{BridgeURL: "ws://localhost:3001", AllowFrom: []string{}}
}
