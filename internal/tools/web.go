package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/go-shiori/go-readability"
)

const (
	webUserAgent   = "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_7_2) AppleWebKit/537.36"
	maxRedirects   = 5
)

// validateURL checks that url is http(s) with a valid domain.
func validateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("only http/https allowed, got %q", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("missing domain in URL")
	}
	return nil
}

// ---------------------------------------------------------------------------
// WebSearchTool
// ---------------------------------------------------------------------------

// WebSearchTool searches the web using the Brave Search API.
type WebSearchTool struct {
	apiKey     string
	maxResults int
	httpClient *http.Client
}

// NewWebSearchTool creates a WebSearchTool.
// apiKey is BRAVE_API_KEY; maxResults defaults to 5.
func NewWebSearchTool(apiKey string, maxResults int) *WebSearchTool {
	if maxResults <= 0 {
		maxResults = 5
	}
	return &WebSearchTool{
		apiKey:     apiKey,
		maxResults: maxResults,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (t *WebSearchTool) Name() string        { return "web_search" }
func (t *WebSearchTool) Description() string { return "Search the web. Returns titles, URLs, and snippets." }
func (t *WebSearchTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "Search query"
			},
			"count": {
				"type": "integer",
				"description": "Results (1-10)",
				"minimum": 1,
				"maximum": 10
			}
		},
		"required": ["query"]
	}`)
}

func (t *WebSearchTool) Execute(ctx context.Context, params map[string]any) (string, error) {
	if t.apiKey == "" {
		return "Error: BRAVE_API_KEY not configured", nil
	}
	query, _ := params["query"].(string)
	if query == "" {
		return "Error: query is required", nil
	}

	n := t.maxResults
	if countVal, ok := params["count"]; ok {
		switch v := countVal.(type) {
		case float64:
			n = int(v)
		case int:
			n = v
		}
	}
	if n < 1 {
		n = 1
	}
	if n > 10 {
		n = 10
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.search.brave.com/res/v1/web/search", nil)
	if err != nil {
		return fmt.Sprintf("Error: %v", err), nil
	}
	q := req.URL.Query()
	q.Set("q", query)
	q.Set("count", fmt.Sprintf("%d", n))
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", t.apiKey)

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return fmt.Sprintf("Error: %v", err), nil
	}
	defer resp.Body.Close()

	var data struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return fmt.Sprintf("Error parsing response: %v", err), nil
	}

	results := data.Web.Results
	if len(results) == 0 {
		return fmt.Sprintf("No results for: %s", query), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Results for: %s\n\n", query))
	for i, item := range results {
		if i >= n {
			break
		}
		sb.WriteString(fmt.Sprintf("%d. %s\n   %s", i+1, item.Title, item.URL))
		if item.Description != "" {
			sb.WriteString("\n   " + item.Description)
		}
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

// ---------------------------------------------------------------------------
// WebFetchTool
// ---------------------------------------------------------------------------

// WebFetchTool fetches a URL and extracts readable content.
type WebFetchTool struct {
	maxChars   int
	httpClient *http.Client
}

// NewWebFetchTool creates a WebFetchTool. maxChars defaults to 50000.
func NewWebFetchTool(maxChars int) *WebFetchTool {
	if maxChars <= 0 {
		maxChars = 50000
	}
	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("stopped after %d redirects", maxRedirects)
			}
			return nil
		},
	}
	return &WebFetchTool{maxChars: maxChars, httpClient: client}
}

func (t *WebFetchTool) Name() string { return "web_fetch" }
func (t *WebFetchTool) Description() string {
	return "Fetch URL and extract readable content (HTML → markdown/text)."
}
func (t *WebFetchTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"url": {
				"type": "string",
				"description": "URL to fetch"
			},
			"extractMode": {
				"type": "string",
				"enum": ["markdown", "text"],
				"default": "markdown"
			},
			"maxChars": {
				"type": "integer",
				"minimum": 100
			}
		},
		"required": ["url"]
	}`)
}

func (t *WebFetchTool) Execute(ctx context.Context, params map[string]any) (string, error) {
	rawURL, _ := params["url"].(string)
	if rawURL == "" {
		return "Error: url is required", nil
	}

	if err := validateURL(rawURL); err != nil {
		result, _ := json.Marshal(map[string]any{
			"error": fmt.Sprintf("URL validation failed: %v", err),
			"url":   rawURL,
		})
		return string(result), nil
	}

	extractMode := "markdown"
	if m, ok := params["extractMode"].(string); ok && m != "" {
		extractMode = m
	}
	maxChars := t.maxChars
	if mc, ok := params["maxChars"]; ok {
		switch v := mc.(type) {
		case float64:
			maxChars = int(v)
		case int:
			maxChars = v
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		out, _ := json.Marshal(map[string]any{"error": err.Error(), "url": rawURL})
		return string(out), nil
	}
	req.Header.Set("User-Agent", webUserAgent)

	resp, err := t.httpClient.Do(req)
	if err != nil {
		out, _ := json.Marshal(map[string]any{"error": err.Error(), "url": rawURL})
		return string(out), nil
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		out, _ := json.Marshal(map[string]any{"error": err.Error(), "url": rawURL})
		return string(out), nil
	}

	ctype := resp.Header.Get("Content-Type")
	finalURL := resp.Request.URL.String()

	var text, extractor string

	switch {
	case strings.Contains(ctype, "application/json"):
		var jsonData any
		if err := json.Unmarshal(bodyBytes, &jsonData); err == nil {
			formatted, _ := json.MarshalIndent(jsonData, "", "  ")
			text = string(formatted)
		} else {
			text = string(bodyBytes)
		}
		extractor = "json"

	case strings.Contains(ctype, "text/html") || isHTMLPrefix(bodyBytes):
		parsedURL, _ := url.Parse(rawURL)
		article, err := readability.FromReader(bytes.NewReader(bodyBytes), parsedURL)
		if err == nil {
			if extractMode == "markdown" {
				text = htmlToMarkdown(article.Content)
			} else {
				text = stripHTMLTags(article.Content)
			}
			if article.Title != "" {
				text = "# " + article.Title + "\n\n" + text
			}
		} else {
			// Fallback: just strip tags
			text = stripHTMLTags(string(bodyBytes))
		}
		extractor = "readability"

	default:
		text = string(bodyBytes)
		extractor = "raw"
	}

	truncated := len(text) > maxChars
	if truncated {
		text = text[:maxChars]
	}

	out, _ := json.Marshal(map[string]any{
		"url":       rawURL,
		"finalUrl":  finalURL,
		"status":    resp.StatusCode,
		"extractor": extractor,
		"truncated": truncated,
		"length":    len(text),
		"text":      text,
	})
	return string(out), nil
}

// isHTMLPrefix returns true if the body starts with an HTML declaration.
func isHTMLPrefix(b []byte) bool {
	prefix := strings.ToLower(strings.TrimSpace(string(b[:min(256, len(b))])))
	return strings.HasPrefix(prefix, "<!doctype") || strings.HasPrefix(prefix, "<html")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ---------------------------------------------------------------------------
// HTML → text/markdown helpers
// ---------------------------------------------------------------------------

var (
	reScript    = regexp.MustCompile(`(?is)<script[\s\S]*?</script>`)
	reStyle     = regexp.MustCompile(`(?is)<style[\s\S]*?</style>`)
	reTags      = regexp.MustCompile(`<[^>]+>`)
	reSpaces    = regexp.MustCompile(`[ \t]+`)
	reNewlines  = regexp.MustCompile(`\n{3,}`)
	reLinks     = regexp.MustCompile(`(?is)<a\s+[^>]*href=["']([^"']+)["'][^>]*>([\s\S]*?)</a>`)
	reHeadings  = regexp.MustCompile(`(?is)<h([1-6])[^>]*>([\s\S]*?)</h[1-6]>`)
	reListItems = regexp.MustCompile(`(?is)<li[^>]*>([\s\S]*?)</li>`)
	reBlockEnd  = regexp.MustCompile(`(?is)</(p|div|section|article)>`)
	reLineBreak = regexp.MustCompile(`(?is)<(br|hr)\s*/?>`)
)

// stripHTMLTags removes all HTML tags and normalizes whitespace.
func stripHTMLTags(text string) string {
	text = reScript.ReplaceAllString(text, "")
	text = reStyle.ReplaceAllString(text, "")
	text = reTags.ReplaceAllString(text, "")
	text = reSpaces.ReplaceAllString(text, " ")
	text = reNewlines.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}

// htmlToMarkdown converts HTML to a simple markdown representation.
// Mirrors Python WebFetchTool._to_markdown().
func htmlToMarkdown(htmlText string) string {
	// Links
	text := reLinks.ReplaceAllStringFunc(htmlText, func(m string) string {
		parts := reLinks.FindStringSubmatch(m)
		if len(parts) < 3 {
			return m
		}
		return fmt.Sprintf("[%s](%s)", stripHTMLTags(parts[2]), parts[1])
	})
	// Headings
	text = reHeadings.ReplaceAllStringFunc(text, func(m string) string {
		parts := reHeadings.FindStringSubmatch(m)
		if len(parts) < 3 {
			return m
		}
		level := len(parts[1]) // "1".."6" — actually string digit
		hashes := strings.Repeat("#", level)
		return fmt.Sprintf("\n%s %s\n", hashes, stripHTMLTags(parts[2]))
	})
	// List items
	text = reListItems.ReplaceAllStringFunc(text, func(m string) string {
		parts := reListItems.FindStringSubmatch(m)
		if len(parts) < 2 {
			return m
		}
		return "\n- " + stripHTMLTags(parts[1])
	})
	// Block endings → paragraph break
	text = reBlockEnd.ReplaceAllString(text, "\n\n")
	// Line breaks
	text = reLineBreak.ReplaceAllString(text, "\n")
	return normalizeWhitespace(stripHTMLTags(text))
}

func normalizeWhitespace(text string) string {
	text = reSpaces.ReplaceAllString(text, " ")
	text = reNewlines.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}
