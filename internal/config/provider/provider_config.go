package provider

const (
	ProviderCustom        = "custom"
	ProviderAnthropic     = "anthropic"
	ProviderOpenAI        = "openai"
	ProviderOpenRouter    = "openrouter"
	ProviderDeepSeek      = "deepseek"
	ProviderGroq          = "groq"
	ProviderZhipu         = "zhipu"
	ProviderDashScope     = "dashscope"
	ProviderVLLM          = "vllm"
	ProviderGemini        = "gemini"
	ProviderMoonshot      = "moonshot"
	ProviderMiniMax       = "minimax"
	ProviderAiHubMix      = "aihubmix"
	ProviderSiliconFlow   = "siliconflow"
	ProviderVolcEngine    = "volcengine"
	ProviderOpenAICodex   = "openai_codex"
	ProviderGithubCopilot = "github_copilot"
)

// ProviderConfig holds credentials for one LLM provider.
type ProviderConfig struct {
	APIKey       string            `json:"apiKey"`
	APIBase      string            `json:"apiBase,omitempty"`
	ExtraHeaders map[string]string `json:"extraHeaders,omitempty"`
}

// ProvidersConfig holds credentials for all supported LLM providers.
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

func DefaultProvidersConfig() ProvidersConfig {
	return ProvidersConfig{}
}

// ByName returns a pointer to the ProviderConfig field matching the given
// registry name. Returns nil if the name is unknown.
func (p *ProvidersConfig) ByName(name string) *ProviderConfig {
	switch name {
	case ProviderCustom:
		return &p.Custom
	case ProviderAnthropic:
		return &p.Anthropic
	case ProviderOpenAI:
		return &p.OpenAI
	case ProviderOpenRouter:
		return &p.OpenRouter
	case ProviderDeepSeek:
		return &p.DeepSeek
	case ProviderGroq:
		return &p.Groq
	case ProviderZhipu:
		return &p.Zhipu
	case ProviderDashScope:
		return &p.DashScope
	case ProviderVLLM:
		return &p.VLLM
	case ProviderGemini:
		return &p.Gemini
	case ProviderMoonshot:
		return &p.Moonshot
	case ProviderMiniMax:
		return &p.MiniMax
	case ProviderAiHubMix:
		return &p.AiHubMix
	case ProviderSiliconFlow:
		return &p.SiliconFlow
	case ProviderVolcEngine:
		return &p.VolcEngine
	case ProviderOpenAICodex:
		return &p.OpenAICodex
	case ProviderGithubCopilot:
		return &p.GithubCopilot
	}
	return nil
}
