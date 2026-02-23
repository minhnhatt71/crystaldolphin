package tool

// WebSearchConfig configures the Brave web-search tool.
type WebSearchConfig struct {
	APIKey     string `json:"apiKey"`
	MaxResults int    `json:"maxResults"`
}

func DefaultWebSearchConfig() WebSearchConfig {
	return WebSearchConfig{MaxResults: 5}
}

// WebToolsConfig groups web-related tool settings.
type WebToolsConfig struct {
	Search WebSearchConfig `json:"search"`
}

func DefaultWebToolsConfig() WebToolsConfig {
	return WebToolsConfig{Search: DefaultWebSearchConfig()}
}
