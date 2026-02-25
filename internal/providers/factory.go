package providers

import "github.com/crystaldolphin/crystaldolphin/internal/schema"

// Params are the raw values needed to construct any schema.LLMProvider.
// Extracted from config.Config by the caller to avoid an import cycle.
type Params struct {
	APIKey       string
	APIBase      string
	ExtraHeaders map[string]string
	DefaultModel string
	ProviderName string // registry name, e.g. "openrouter", "anthropic"
}

// New creates the appropriate schema.LLMProvider for the given params.
//
// Rules (mirrors Python's _make_provider):
//   - openai_codex → CodexProvider (OAuth + SSE)
//   - otherwise    → OpenAIProvider (direct HTTP, handles all OpenAI-compat providers
//                    including Anthropic native API)
func New(p Params) schema.LLMProvider {
	if p.ProviderName == "openai_codex" ||
		p.ProviderName == "openai-codex" {
		return NewCodexProvider(p.DefaultModel)
	}
	return NewOpenAIProvider(p.APIKey, p.APIBase, p.DefaultModel, p.ProviderName, p.ExtraHeaders)
}
