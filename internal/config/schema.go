// Package config defines the configuration schema for crystaldolphin.
//
// JSON keys use camelCase to stay byte-compatible with existing
// ~/.nanobot/config.json files created by the Python nanobot.
package config

import (
	"os"
	"path/filepath"
)

// ProviderConfig holds credentials for one LLM provider.
type ProviderConfig struct {
	APIKey       string            `json:"apiKey"`
	APIBase      string            `json:"apiBase,omitempty"`
	ExtraHeaders map[string]string `json:"extraHeaders,omitempty"`
}

// ProvidersConfig holds credentials for all supported LLM providers.
// Field names match nanobot's ProvidersConfig exactly.
type ProvidersConfig struct {
	Custom        ProviderConfig `json:"custom"`
	Anthropic     ProviderConfig `json:"anthropic"`
	OpenAI        ProviderConfig `json:"openai"`
	OpenRouter    ProviderConfig `json:"openrouter"`
	DeepSeek      ProviderConfig `json:"deepseek"`
	Groq          ProviderConfig `json:"groq"`
	Zhipu         ProviderConfig `json:"zhipu"`
	DashScope     ProviderConfig `json:"dashscope"`
	VLLM          ProviderConfig `json:"vllm"`
	Gemini        ProviderConfig `json:"gemini"`
	Moonshot      ProviderConfig `json:"moonshot"`
	MiniMax       ProviderConfig `json:"minimax"`
	AiHubMix      ProviderConfig `json:"aihubmix"`
	SiliconFlow   ProviderConfig `json:"siliconflow"`
	VolcEngine    ProviderConfig `json:"volcengine"`
	OpenAICodex   ProviderConfig `json:"openaiCodex"`
	GithubCopilot ProviderConfig `json:"githubCopilot"`
}

// AgentDefaults holds default values for agent behaviour.
type AgentDefaults struct {
	Workspace        string  `json:"workspace"`
	Model            string  `json:"model"`
	MaxTokens        int     `json:"maxTokens"`
	Temperature      float64 `json:"temperature"`
	MaxToolIter      int     `json:"maxToolIterations"`
	MemoryWindow     int     `json:"memoryWindow"`
}

func defaultAgentDefaults() AgentDefaults {
	return AgentDefaults{
		Workspace:    "~/.nanobot/workspace",
		Model:        "anthropic/claude-opus-4-5",
		MaxTokens:    8192,
		Temperature:  0.7,
		MaxToolIter:  20,
		MemoryWindow: 50,
	}
}

// AgentsConfig wraps agent defaults (mirrors nanobot's AgentsConfig).
type AgentsConfig struct {
	Defaults AgentDefaults `json:"defaults"`
}

func defaultAgentsConfig() AgentsConfig {
	return AgentsConfig{Defaults: defaultAgentDefaults()}
}

// ---- Channel configs -------------------------------------------------------

// WhatsAppConfig configures the WhatsApp channel.
type WhatsAppConfig struct {
	Enabled     bool     `json:"enabled"`
	BridgeURL   string   `json:"bridgeUrl"`
	BridgeToken string   `json:"bridgeToken"`
	AllowFrom   []string `json:"allowFrom"`
}

func defaultWhatsAppConfig() WhatsAppConfig {
	return WhatsAppConfig{BridgeURL: "ws://localhost:3001", AllowFrom: []string{}}
}

// TelegramConfig configures the Telegram channel.
type TelegramConfig struct {
	Enabled        bool     `json:"enabled"`
	Token          string   `json:"token"`
	AllowFrom      []string `json:"allowFrom"`
	Proxy          string   `json:"proxy,omitempty"`
	ReplyToMessage bool     `json:"replyToMessage"`
}

func defaultTelegramConfig() TelegramConfig {
	return TelegramConfig{AllowFrom: []string{}}
}

// FeishuConfig configures the Feishu/Lark channel.
type FeishuConfig struct {
	Enabled           bool     `json:"enabled"`
	AppID             string   `json:"appId"`
	AppSecret         string   `json:"appSecret"`
	EncryptKey        string   `json:"encryptKey"`
	VerificationToken string   `json:"verificationToken"`
	AllowFrom         []string `json:"allowFrom"`
}

func defaultFeishuConfig() FeishuConfig {
	return FeishuConfig{AllowFrom: []string{}}
}

// DingTalkConfig configures the DingTalk channel.
type DingTalkConfig struct {
	Enabled      bool     `json:"enabled"`
	ClientID     string   `json:"clientId"`
	ClientSecret string   `json:"clientSecret"`
	AllowFrom    []string `json:"allowFrom"`
}

func defaultDingTalkConfig() DingTalkConfig {
	return DingTalkConfig{AllowFrom: []string{}}
}

// DiscordConfig configures the Discord channel.
type DiscordConfig struct {
	Enabled    bool     `json:"enabled"`
	Token      string   `json:"token"`
	AllowFrom  []string `json:"allowFrom"`
	GatewayURL string   `json:"gatewayUrl"`
	Intents    int      `json:"intents"`
}

func defaultDiscordConfig() DiscordConfig {
	return DiscordConfig{
		GatewayURL: "wss://gateway.discord.gg/?v=10&encoding=json",
		Intents:    37377, // GUILDS + GUILD_MESSAGES + DIRECT_MESSAGES + MESSAGE_CONTENT
		AllowFrom:  []string{},
	}
}

// EmailConfig configures the email channel (IMAP inbound + SMTP outbound).
type EmailConfig struct {
	Enabled       bool     `json:"enabled"`
	ConsentGranted bool    `json:"consentGranted"`

	// IMAP (receive)
	IMAPHost     string `json:"imapHost"`
	IMAPPort     int    `json:"imapPort"`
	IMAPUsername string `json:"imapUsername"`
	IMAPPassword string `json:"imapPassword"`
	IMAPMailbox  string `json:"imapMailbox"`
	IMAPUseSSL   bool   `json:"imapUseSsl"`

	// SMTP (send)
	SMTPHost     string `json:"smtpHost"`
	SMTPPort     int    `json:"smtpPort"`
	SMTPUsername string `json:"smtpUsername"`
	SMTPPassword string `json:"smtpPassword"`
	SMTPUseTLS   bool   `json:"smtpUseTls"`
	SMTPUseSSL   bool   `json:"smtpUseSsl"`
	FromAddress  string `json:"fromAddress"`

	// Behaviour
	AutoReplyEnabled    bool     `json:"autoReplyEnabled"`
	PollIntervalSeconds int      `json:"pollIntervalSeconds"`
	MarkSeen            bool     `json:"markSeen"`
	MaxBodyChars        int      `json:"maxBodyChars"`
	SubjectPrefix       string   `json:"subjectPrefix"`
	AllowFrom           []string `json:"allowFrom"`
}

func defaultEmailConfig() EmailConfig {
	return EmailConfig{
		IMAPPort:            993,
		IMAPMailbox:         "INBOX",
		IMAPUseSSL:          true,
		SMTPPort:            587,
		SMTPUseTLS:          true,
		AutoReplyEnabled:    true,
		PollIntervalSeconds: 30,
		MarkSeen:            true,
		MaxBodyChars:        12000,
		SubjectPrefix:       "Re: ",
		AllowFrom:           []string{},
	}
}

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
	Enabled                  bool                       `json:"enabled"`
	BaseURL                  string                     `json:"baseUrl"`
	SocketURL                string                     `json:"socketUrl"`
	SocketPath               string                     `json:"socketPath"`
	SocketDisableMsgpack     bool                       `json:"socketDisableMsgpack"`
	SocketReconnectDelayMs   int                        `json:"socketReconnectDelayMs"`
	SocketMaxReconnectDelayMs int                       `json:"socketMaxReconnectDelayMs"`
	SocketConnectTimeoutMs   int                        `json:"socketConnectTimeoutMs"`
	RefreshIntervalMs        int                        `json:"refreshIntervalMs"`
	WatchTimeoutMs           int                        `json:"watchTimeoutMs"`
	WatchLimit               int                        `json:"watchLimit"`
	RetryDelayMs             int                        `json:"retryDelayMs"`
	MaxRetryAttempts         int                        `json:"maxRetryAttempts"`
	ClawToken                string                     `json:"clawToken"`
	AgentUserID              string                     `json:"agentUserId"`
	Sessions                 []string                   `json:"sessions"`
	Panels                   []string                   `json:"panels"`
	AllowFrom                []string                   `json:"allowFrom"`
	Mention                  MochatMentionConfig        `json:"mention"`
	Groups                   map[string]MochatGroupRule `json:"groups"`
	ReplyDelayMode           string                     `json:"replyDelayMode"`
	ReplyDelayMs             int                        `json:"replyDelayMs"`
}

func defaultMochatConfig() MochatConfig {
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

// SlackDMConfig controls direct-message behaviour in Slack.
type SlackDMConfig struct {
	Enabled   bool     `json:"enabled"`
	Policy    string   `json:"policy"` // "open" or "allowlist"
	AllowFrom []string `json:"allowFrom"`
}

func defaultSlackDMConfig() SlackDMConfig {
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

func defaultSlackConfig() SlackConfig {
	return SlackConfig{
		Mode:          "socket",
		WebhookPath:   "/slack/events",
		UserTokenReadOnly: true,
		ReplyInThread: true,
		ReactEmoji:    "eyes",
		GroupPolicy:   "mention",
		GroupAllowFrom: []string{},
		DM:            defaultSlackDMConfig(),
	}
}

// QQConfig configures the QQ channel.
type QQConfig struct {
	Enabled   bool     `json:"enabled"`
	AppID     string   `json:"appId"`
	Secret    string   `json:"secret"`
	AllowFrom []string `json:"allowFrom"`
}

func defaultQQConfig() QQConfig {
	return QQConfig{AllowFrom: []string{}}
}

// ChannelsConfig groups all channel configurations.
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

func defaultChannelsConfig() ChannelsConfig {
	return ChannelsConfig{
		WhatsApp: defaultWhatsAppConfig(),
		Telegram: defaultTelegramConfig(),
		Discord:  defaultDiscordConfig(),
		Feishu:   defaultFeishuConfig(),
		Mochat:   defaultMochatConfig(),
		DingTalk: defaultDingTalkConfig(),
		Email:    defaultEmailConfig(),
		Slack:    defaultSlackConfig(),
		QQ:       defaultQQConfig(),
	}
}

// ---- Tool configs ----------------------------------------------------------

// WebSearchConfig configures the Brave web-search tool.
type WebSearchConfig struct {
	APIKey     string `json:"apiKey"`
	MaxResults int    `json:"maxResults"`
}

func defaultWebSearchConfig() WebSearchConfig {
	return WebSearchConfig{MaxResults: 5}
}

// WebToolsConfig groups web-related tool settings.
type WebToolsConfig struct {
	Search WebSearchConfig `json:"search"`
}

func defaultWebToolsConfig() WebToolsConfig {
	return WebToolsConfig{Search: defaultWebSearchConfig()}
}

// ExecToolConfig configures the shell-exec tool.
type ExecToolConfig struct {
	Timeout int `json:"timeout"` // seconds
}

func defaultExecToolConfig() ExecToolConfig {
	return ExecToolConfig{Timeout: 60}
}

// MCPServerConfig describes one MCP server connection (stdio or HTTP).
type MCPServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

// ToolsConfig groups all tool-level settings.
type ToolsConfig struct {
	Web                WebToolsConfig             `json:"web"`
	Exec               ExecToolConfig             `json:"exec"`
	RestrictToWorkspace bool                      `json:"restrictToWorkspace"`
	MCPServers         map[string]MCPServerConfig `json:"mcpServers"`
}

func defaultToolsConfig() ToolsConfig {
	return ToolsConfig{
		Web:        defaultWebToolsConfig(),
		Exec:       defaultExecToolConfig(),
		MCPServers: map[string]MCPServerConfig{},
	}
}

// GatewayConfig holds gateway server settings.
type GatewayConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

func defaultGatewayConfig() GatewayConfig {
	return GatewayConfig{Host: "0.0.0.0", Port: 18790}
}

// ---- Root config -----------------------------------------------------------

// Config is the root configuration object, loaded from ~/.nanobot/config.json.
type Config struct {
	Agents    AgentsConfig   `json:"agents"`
	Channels  ChannelsConfig `json:"channels"`
	Providers ProvidersConfig `json:"providers"`
	Gateway   GatewayConfig  `json:"gateway"`
	Tools     ToolsConfig    `json:"tools"`
}

// DefaultConfig returns a Config populated with all default values.
func DefaultConfig() Config {
	return Config{
		Agents:    defaultAgentsConfig(),
		Channels:  defaultChannelsConfig(),
		Providers: ProvidersConfig{},
		Gateway:   defaultGatewayConfig(),
		Tools:     defaultToolsConfig(),
	}
}

// WorkspacePath returns the expanded absolute path to the agent workspace.
func (c *Config) WorkspacePath() string {
	ws := c.Agents.Defaults.Workspace
	if ws == "" {
		ws = "~/.nanobot/workspace"
	}
	if len(ws) >= 2 && ws[:2] == "~/" {
		home, err := os.UserHomeDir()
		if err == nil {
			ws = filepath.Join(home, ws[2:])
		}
	}
	return ws
}

// ProviderByName returns a pointer to the ProviderConfig field matching the
// given registry name (e.g. "openrouter", "anthropic"). Returns nil if unknown.
func (c *Config) ProviderByName(name string) *ProviderConfig {
	switch name {
	case "custom":
		return &c.Providers.Custom
	case "anthropic":
		return &c.Providers.Anthropic
	case "openai":
		return &c.Providers.OpenAI
	case "openrouter":
		return &c.Providers.OpenRouter
	case "deepseek":
		return &c.Providers.DeepSeek
	case "groq":
		return &c.Providers.Groq
	case "zhipu":
		return &c.Providers.Zhipu
	case "dashscope":
		return &c.Providers.DashScope
	case "vllm":
		return &c.Providers.VLLM
	case "gemini":
		return &c.Providers.Gemini
	case "moonshot":
		return &c.Providers.Moonshot
	case "minimax":
		return &c.Providers.MiniMax
	case "aihubmix":
		return &c.Providers.AiHubMix
	case "siliconflow":
		return &c.Providers.SiliconFlow
	case "volcengine":
		return &c.Providers.VolcEngine
	case "openai_codex":
		return &c.Providers.OpenAICodex
	case "github_copilot":
		return &c.Providers.GithubCopilot
	}
	return nil
}
