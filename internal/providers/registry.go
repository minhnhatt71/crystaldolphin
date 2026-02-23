package providers

import "strings"

// ModelOverride applies extra parameters for a specific model pattern.
type ModelOverride struct {
	Pattern   string         // case-insensitive substring to match in model name
	Overrides map[string]any // parameters to merge into the request body
}

// ProviderSpec is the metadata record for one LLM provider.
// Translated verbatim from nanobot's Python ProviderSpec dataclass.
type ProviderSpec struct {
	// Identity
	Name        string   // config field name, e.g. "dashscope"
	Keywords    []string // model-name keywords for matching (lowercase)
	EnvKey      string   // env var for the API key (LiteLLM compat, unused in Go direct calls)
	DisplayName string   // shown in `crystaldolphin status`

	// Model prefixing (used in resolveModel)
	LiteLLMPrefix string   // prefix added to model names for routing
	SkipPrefixes  []string // don't add prefix if model already starts with one of these

	// Extra env vars to set: each is [envName, valueTemplate]
	// Templates: {api_key} and {api_base} are substituted at construction.
	EnvExtras [][2]string

	// Gateway / local detection
	IsGateway           bool   // routes any model (OpenRouter, AiHubMix, …)
	IsLocal             bool   // local deployment (vLLM)
	DetectByKeyPrefix   string // match api_key prefix to identify gateway
	DetectByBaseKeyword string // match substring in api_base URL
	DefaultAPIBase      string // fallback base URL when none is configured

	// Gateway behaviour
	StripModelPrefix bool // strip "provider/" before using the model name

	// Per-model parameter overrides
	ModelOverrides []ModelOverride

	// OAuth-based (no API key; use device flow instead)
	IsOAuth bool

	// Direct provider (bypass LiteLLM-style routing)
	IsDirect bool

	// Provider supports cache_control on content blocks (Anthropic prompt caching)
	SupportsPromptCaching bool
}

// Label returns the display name, defaulting to Title-cased Name.
func (s ProviderSpec) Label() string {
	if s.DisplayName != "" {
		return s.DisplayName
	}
	return strings.ToTitle(s.Name[:1]) + s.Name[1:]
}

// ---------------------------------------------------------------------------
// PROVIDERS — the registry.  Order = match priority.
// Mirrors nanobot's Python PROVIDERS tuple exactly.
// ---------------------------------------------------------------------------

var PROVIDERS = []ProviderSpec{
	{
		Name:        "custom",
		Keywords:    nil,
		EnvKey:      "",
		DisplayName: "Custom",
		IsDirect:    true,
	},
	{
		Name:                  "openrouter",
		Keywords:              []string{"openrouter"},
		EnvKey:                "OPENROUTER_API_KEY",
		DisplayName:           "OpenRouter",
		LiteLLMPrefix:         "openrouter",
		IsGateway:             true,
		DetectByKeyPrefix:     "sk-or-",
		DetectByBaseKeyword:   "openrouter",
		DefaultAPIBase:        "https://openrouter.ai/api/v1",
		SupportsPromptCaching: true,
	},
	{
		Name:                "aihubmix",
		Keywords:            []string{"aihubmix"},
		EnvKey:              "OPENAI_API_KEY",
		DisplayName:         "AiHubMix",
		LiteLLMPrefix:       "openai",
		IsGateway:           true,
		DetectByBaseKeyword: "aihubmix",
		DefaultAPIBase:      "https://aihubmix.com/v1",
		StripModelPrefix:    true,
	},
	{
		Name:                "siliconflow",
		Keywords:            []string{"siliconflow"},
		EnvKey:              "OPENAI_API_KEY",
		DisplayName:         "SiliconFlow",
		LiteLLMPrefix:       "openai",
		IsGateway:           true,
		DetectByBaseKeyword: "siliconflow",
		DefaultAPIBase:      "https://api.siliconflow.cn/v1",
	},
	{
		Name:                "volcengine",
		Keywords:            []string{"volcengine", "volces", "ark"},
		EnvKey:              "OPENAI_API_KEY",
		DisplayName:         "VolcEngine",
		LiteLLMPrefix:       "volcengine",
		IsGateway:           true,
		DetectByBaseKeyword: "volces",
		DefaultAPIBase:      "https://ark.cn-beijing.volces.com/api/v3",
	},
	{
		Name:                  "anthropic",
		Keywords:              []string{"anthropic", "claude"},
		EnvKey:                "ANTHROPIC_API_KEY",
		DisplayName:           "Anthropic",
		SupportsPromptCaching: true,
	},
	{
		Name:        "openai",
		Keywords:    []string{"openai", "gpt"},
		EnvKey:      "OPENAI_API_KEY",
		DisplayName: "OpenAI",
	},
	{
		Name:           "openai_codex",
		Keywords:       []string{"openai-codex", "codex"},
		EnvKey:         "",
		DisplayName:    "OpenAI Codex",
		DefaultAPIBase: "https://chatgpt.com/backend-api",
		IsOAuth:        true,
	},
	{
		Name:          "github_copilot",
		Keywords:      []string{"github_copilot", "copilot"},
		EnvKey:        "",
		DisplayName:   "Github Copilot",
		LiteLLMPrefix: "github_copilot",
		SkipPrefixes:  []string{"github_copilot/"},
		IsOAuth:       true,
	},
	{
		Name:          "deepseek",
		Keywords:      []string{"deepseek"},
		EnvKey:        "DEEPSEEK_API_KEY",
		DisplayName:   "DeepSeek",
		LiteLLMPrefix: "deepseek",
		SkipPrefixes:  []string{"deepseek/"},
	},
	{
		Name:          "gemini",
		Keywords:      []string{"gemini"},
		EnvKey:        "GEMINI_API_KEY",
		DisplayName:   "Gemini",
		LiteLLMPrefix: "gemini",
		SkipPrefixes:  []string{"gemini/"},
	},
	{
		Name:          "zhipu",
		Keywords:      []string{"zhipu", "glm", "zai"},
		EnvKey:        "ZAI_API_KEY",
		DisplayName:   "Zhipu AI",
		LiteLLMPrefix: "zai",
		SkipPrefixes:  []string{"zhipu/", "zai/", "openrouter/", "hosted_vllm/"},
		EnvExtras:     [][2]string{{"ZHIPUAI_API_KEY", "{api_key}"}},
	},
	{
		Name:          "dashscope",
		Keywords:      []string{"qwen", "dashscope"},
		EnvKey:        "DASHSCOPE_API_KEY",
		DisplayName:   "DashScope",
		LiteLLMPrefix: "dashscope",
		SkipPrefixes:  []string{"dashscope/", "openrouter/"},
	},
	{
		Name:           "moonshot",
		Keywords:       []string{"moonshot", "kimi"},
		EnvKey:         "MOONSHOT_API_KEY",
		DisplayName:    "Moonshot",
		LiteLLMPrefix:  "moonshot",
		SkipPrefixes:   []string{"moonshot/", "openrouter/"},
		EnvExtras:      [][2]string{{"MOONSHOT_API_BASE", "{api_base}"}},
		DefaultAPIBase: "https://api.moonshot.ai/v1",
		ModelOverrides: []ModelOverride{
			{Pattern: "kimi-k2.5", Overrides: map[string]any{"temperature": 1.0}},
		},
	},
	{
		Name:           "minimax",
		Keywords:       []string{"minimax"},
		EnvKey:         "MINIMAX_API_KEY",
		DisplayName:    "MiniMax",
		LiteLLMPrefix:  "minimax",
		SkipPrefixes:   []string{"minimax/", "openrouter/"},
		DefaultAPIBase: "https://api.minimax.io/v1",
	},
	{
		Name:          "vllm",
		Keywords:      []string{"vllm"},
		EnvKey:        "HOSTED_VLLM_API_KEY",
		DisplayName:   "vLLM/Local",
		LiteLLMPrefix: "hosted_vllm",
		IsLocal:       true,
	},
	{
		Name:          "groq",
		Keywords:      []string{"groq"},
		EnvKey:        "GROQ_API_KEY",
		DisplayName:   "Groq",
		LiteLLMPrefix: "groq",
		SkipPrefixes:  []string{"groq/"},
	},
}

// FindByModel matches a standard provider by model-name keyword (case-insensitive).
// Skips gateways and local providers — those are matched by api_key/api_base.
// Mirrors Python's find_by_model().
func FindByModel(model string) *ProviderSpec {
	modelLower := strings.ToLower(model)
	modelNorm := strings.ReplaceAll(modelLower, "-", "_")
	modelPrefix, _, _ := strings.Cut(modelLower, "/")
	normalizedPrefix := strings.ReplaceAll(modelPrefix, "-", "_")

	// Collect non-gateway, non-local specs.
	var std []int
	for i := range PROVIDERS {
		if !PROVIDERS[i].IsGateway && !PROVIDERS[i].IsLocal {
			std = append(std, i)
		}
	}

	// Prefer explicit provider prefix.
	for _, i := range std {
		spec := &PROVIDERS[i]
		if modelPrefix != "" && normalizedPrefix == spec.Name {
			return spec
		}
	}

	// Keyword match.
	for _, i := range std {
		spec := &PROVIDERS[i]
		for _, kw := range spec.Keywords {
			kw = strings.ToLower(kw)
			kwNorm := strings.ReplaceAll(kw, "-", "_")
			if strings.Contains(modelLower, kw) || strings.Contains(modelNorm, kwNorm) {
				return spec
			}
		}
	}
	return nil
}

// FindGateway detects the gateway or local provider.
// Priority: (1) explicit provider_name, (2) api_key prefix, (3) api_base keyword.
// Mirrors Python's find_gateway().
func FindGateway(providerName, apiKey, apiBase string) *ProviderSpec {
	// Direct match by config key.
	if providerName != "" {
		if s := FindByName(providerName); s != nil && (s.IsGateway || s.IsLocal) {
			return s
		}
	}
	// Auto-detect by api_key prefix / api_base keyword.
	for i := range PROVIDERS {
		spec := &PROVIDERS[i]
		if spec.DetectByKeyPrefix != "" && strings.HasPrefix(apiKey, spec.DetectByKeyPrefix) {
			return spec
		}
		if spec.DetectByBaseKeyword != "" && strings.Contains(apiBase, spec.DetectByBaseKeyword) {
			return spec
		}
	}
	return nil
}

// FindByName returns the ProviderSpec whose Name equals name.
func FindByName(name string) *ProviderSpec {
	for i := range PROVIDERS {
		if PROVIDERS[i].Name == name {
			return &PROVIDERS[i]
		}
	}
	return nil
}
