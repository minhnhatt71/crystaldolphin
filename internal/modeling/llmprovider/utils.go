package llmprovider

import "regexp"

var reThink = regexp.MustCompile(`(?s)<think>.*?</think>`)

// StripThink removes <think>…</think> blocks that some models embed.
func StripThink(s string) string {
	return reThink.ReplaceAllString(s, "")
}

// Truncate shortens a string to at most n characters, adding "..." if it was truncated.
func Truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
