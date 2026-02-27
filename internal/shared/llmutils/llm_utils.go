package llmutils

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/crystaldolphin/crystaldolphin/internal/schema"
)

var reThink = regexp.MustCompile(`(?s)<think>.*?</think>`)

// Truncate shortens a string to at most n characters, adding "..." if it was truncated.
func Truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// StripThink removes <think>…</think> blocks that some models embed.
func StripThink(s string) string {
	return reThink.ReplaceAllString(s, "")
}

// StringOrDefault returns s if it's not empty, or def if s is empty.
func StringOrDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// ToolHint generates a short hint string for a list of tool calls, e.g. "search("weather in London")".
func ToolHint(tcs []schema.ToolCallResponse) string {
	parts := make([]string, 0, len(tcs))
	for _, tc := range tcs {
		var firstVal string
		for _, v := range tc.Arguments {
			if s, ok := v.(string); ok {
				firstVal = s
			}
			break
		}
		if firstVal == "" {
			parts = append(parts, tc.Name)
			continue
		}
		if len(firstVal) > 40 {
			firstVal = firstVal[:40] + "…"
		}
		parts = append(parts, fmt.Sprintf("%s(%q)", tc.Name, firstVal))
	}
	return strings.Join(parts, ", ")
}
