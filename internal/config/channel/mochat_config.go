package channel

// MochatMentionConfig controls mention behaviour in Mochat.
type MochatMentionConfig struct {
	RequireInGroups bool `json:"requireInGroups"`
}

// MochatGroupRule is a per-group mention requirement.
type MochatGroupRule struct {
	RequireMention bool `json:"requireMention"`
}

// MochatConfig configures the Mochat channel.
type MochatConfig struct {
	Enabled                   bool                       `json:"enabled"`
	BaseURL                   string                     `json:"baseUrl"`
	SocketURL                 string                     `json:"socketUrl"`
	SocketPath                string                     `json:"socketPath"`
	SocketDisableMsgpack      bool                       `json:"socketDisableMsgpack"`
	SocketReconnectDelayMs    int                        `json:"socketReconnectDelayMs"`
	SocketMaxReconnectDelayMs int                        `json:"socketMaxReconnectDelayMs"`
	SocketConnectTimeoutMs    int                        `json:"socketConnectTimeoutMs"`
	RefreshIntervalMs         int                        `json:"refreshIntervalMs"`
	WatchTimeoutMs            int                        `json:"watchTimeoutMs"`
	WatchLimit                int                        `json:"watchLimit"`
	RetryDelayMs              int                        `json:"retryDelayMs"`
	MaxRetryAttempts          int                        `json:"maxRetryAttempts"`
	ClawToken                 string                     `json:"clawToken"`
	AgentUserID               string                     `json:"agentUserId"`
	Sessions                  []string                   `json:"sessions"`
	Panels                    []string                   `json:"panels"`
	AllowFrom                 []string                   `json:"allowFrom"`
	Mention                   MochatMentionConfig        `json:"mention"`
	Groups                    map[string]MochatGroupRule `json:"groups"`
	ReplyDelayMode            string                     `json:"replyDelayMode"`
	ReplyDelayMs              int                        `json:"replyDelayMs"`
}

func DefaultMochatConfig() MochatConfig {
	return MochatConfig{
		BaseURL:                   "https://mochat.io",
		SocketPath:                "/socket.io",
		SocketReconnectDelayMs:    1000,
		SocketMaxReconnectDelayMs: 10000,
		SocketConnectTimeoutMs:    10000,
		RefreshIntervalMs:         30000,
		WatchTimeoutMs:            25000,
		WatchLimit:                100,
		RetryDelayMs:              500,
		Sessions:                  []string{},
		Panels:                    []string{},
		AllowFrom:                 []string{},
		Groups:                    map[string]MochatGroupRule{},
		ReplyDelayMode:            "non-mention",
		ReplyDelayMs:              120000,
	}
}
