package config

import (
	"strings"

	"github.com/crystaldolphin/crystaldolphin/internal/config/provider"
	"github.com/crystaldolphin/crystaldolphin/internal/providers"
)

// MatchResult is the resolved LLM provider config and registry name for a model.
type MatchResult struct {
	Provider *provider.ProviderConfig
	Name     string // e.g. "openrouter", "anthropic"
}

// MatchProvider resolves which provider config and registry entry to use for model.
// If model is empty, the default model from agents.defaults.model is used.
//
// Priority order (mirrors Python Config._match_provider):
//  1. Explicit provider prefix in model string (e.g. "deepseek/deepseek-chat" → deepseek)
//  2. Keyword match in model name (registry order)
//  3. Fallback: gateways first, then others; OAuth providers are never fallback
func (c *Config) MatchProvider(model string) MatchResult {
	if model == "" {
		model = c.Agents.Defaults.Model
	}
	modelLower := strings.ToLower(model)
	modelNorm := strings.ReplaceAll(modelLower, "-", "_")
	modelPrefix, _, _ := strings.Cut(modelLower, "/")
	normalizedPrefix := strings.ReplaceAll(modelPrefix, "-", "_")

	kwMatches := func(kw string) bool {
		kw = strings.ToLower(kw)
		kwNorm := strings.ReplaceAll(kw, "-", "_")
		return strings.Contains(modelLower, kw) || strings.Contains(modelNorm, kwNorm)
	}

	// 1. Explicit provider prefix wins.
	for _, spec := range providers.PROVIDERS {
		p := c.ProviderByName(spec.Name)
		if p == nil {
			continue
		}
		if modelPrefix != "" && normalizedPrefix == spec.Name {
			if spec.IsOAuth || p.APIKey != "" {
				return MatchResult{Provider: p, Name: spec.Name}
			}
		}
	}

	// 2. Keyword match.
	for _, spec := range providers.PROVIDERS {
		p := c.ProviderByName(spec.Name)
		if p == nil {
			continue
		}
		matched := false
		for _, kw := range spec.Keywords {
			if kwMatches(kw) {
				matched = true
				break
			}
		}
		if matched && (spec.IsOAuth || p.APIKey != "") {
			return MatchResult{Provider: p, Name: spec.Name}
		}
	}

	// 3. Fallback: first configured provider; skip OAuth.
	for _, spec := range providers.PROVIDERS {
		if spec.IsOAuth {
			continue
		}
		p := c.ProviderByName(spec.Name)
		if p != nil && p.APIKey != "" {
			return MatchResult{Provider: p, Name: spec.Name}
		}
	}

	return MatchResult{}
}

// GetProvider returns the matched ProviderConfig for model (or nil).
func (c *Config) GetProvider(model string) *provider.ProviderConfig {
	return c.MatchProvider(model).Provider
}

// GetProviderName returns the registry name of the matched provider (or "").
func (c *Config) GetProviderName(model string) string {
	return c.MatchProvider(model).Name
}

// GetAPIBase resolves the effective API base URL for model.
// Precedence: user-configured api_base > spec.default_api_base (gateways only).
func (c *Config) GetAPIBase(model string) string {
	result := c.MatchProvider(model)
	if result.Provider != nil && result.Provider.APIBase != "" {
		return result.Provider.APIBase
	}
	if result.Name != "" {
		spec := providers.FindByName(result.Name)
		if spec != nil && spec.IsGateway && spec.DefaultAPIBase != "" {
			return spec.DefaultAPIBase
		}
	}
	return ""
}

// GetAPIKey returns the API key for model (or "").
func (c *Config) GetAPIKey(model string) string {
	p := c.GetProvider(model)
	if p != nil {
		return p.APIKey
	}
	return ""
}
