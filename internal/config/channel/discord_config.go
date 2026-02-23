package channel

// DiscordConfig configures the Discord channel.
type DiscordConfig struct {
	Enabled    bool     `json:"enabled"`
	Token      string   `json:"token"`
	AllowFrom  []string `json:"allowFrom"`
	GatewayURL string   `json:"gatewayUrl"`
	Intents    int      `json:"intents"`
}

func DefaultDiscordConfig() DiscordConfig {
	return DiscordConfig{
		GatewayURL: "wss://gateway.discord.gg/?v=10&encoding=json",
		Intents:    37377, // GUILDS + GUILD_MESSAGES + DIRECT_MESSAGES + MESSAGE_CONTENT
		AllowFrom:  []string{},
	}
}
