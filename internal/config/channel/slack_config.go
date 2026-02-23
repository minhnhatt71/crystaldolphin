package channel

// SlackDMConfig controls direct-message behaviour in Slack.
type SlackDMConfig struct {
	Enabled   bool     `json:"enabled"`
	Policy    string   `json:"policy"` // "open" or "allowlist"
	AllowFrom []string `json:"allowFrom"`
}

func DefaultSlackDMConfig() SlackDMConfig {
	return SlackDMConfig{Enabled: true, Policy: "open", AllowFrom: []string{}}
}

// SlackConfig configures the Slack channel.
type SlackConfig struct {
	Enabled           bool          `json:"enabled"`
	Mode              string        `json:"mode"`
	WebhookPath       string        `json:"webhookPath"`
	BotToken          string        `json:"botToken"`
	AppToken          string        `json:"appToken"`
	UserTokenReadOnly bool          `json:"userTokenReadOnly"`
	ReplyInThread     bool          `json:"replyInThread"`
	ReactEmoji        string        `json:"reactEmoji"`
	GroupPolicy       string        `json:"groupPolicy"`
	GroupAllowFrom    []string      `json:"groupAllowFrom"`
	DM                SlackDMConfig `json:"dm"`
}

func DefaultSlackConfig() SlackConfig {
	return SlackConfig{
		Mode:              "socket",
		WebhookPath:       "/slack/events",
		UserTokenReadOnly: true,
		ReplyInThread:     true,
		ReactEmoji:        "eyes",
		GroupPolicy:       "mention",
		GroupAllowFrom:    []string{},
		DM:                DefaultSlackDMConfig(),
	}
}
