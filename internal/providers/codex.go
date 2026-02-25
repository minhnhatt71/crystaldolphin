package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	codexURL        = "https://chatgpt.com/backend-api/codex/responses"
	codexOriginator = "crystaldolphin"
)

// CodexToken is the stored OAuth token for the OpenAI Codex provider.
// Written by `crystaldolphin provider login openai-codex` (Phase 6 CLI).
type CodexToken struct {
	AccountID   string `json:"account_id"`
	AccessToken string `json:"access_token"`
	ExpiresAt   int64  `json:"expires_at,omitempty"`
}

// CodexProvider calls the ChatGPT Codex Responses API using a stored OAuth token.
type CodexProvider struct {
	defaultModel string
	tokenPath    string
	httpClient   *http.Client
}

// NewCodexProvider creates a CodexProvider that reads its token from
// ~/.nanobot/codex_token.json.
func NewCodexProvider(defaultModel string) *CodexProvider {
	home, _ := os.UserHomeDir()
	return &CodexProvider{
		defaultModel: defaultModel,
		tokenPath:    filepath.Join(home, ".nanobot", "codex_token.json"),
		httpClient:   &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *CodexProvider) DefaultModel() string { return p.defaultModel }

// Chat implements LLMProvider using the Codex Responses API (SSE).
func (p *CodexProvider) Chat(
	ctx context.Context,
	messages MessageHistory,
	tools []map[string]any,
	opts ChatOptions,
) (LLMResponse, error) {
	token, err := p.loadToken()
	if err != nil {
		s := fmt.Sprintf("Codex token not found — run `crystaldolphin provider login openai-codex` first: %v", err)
		return LLMResponse{Content: &s, FinishReason: "error"}, nil
	}

	model := opts.Model
	if model == "" {
		model = p.defaultModel
	}
	model = stripCodexPrefix(model)

	system, inputItems := convertMessagesForCodex(messages)

	body := map[string]any{
		"model":               model,
		"store":               false,
		"stream":              true,
		"instructions":        system,
		"input":               inputItems,
		"text":                map[string]any{"verbosity": "medium"},
		"include":             []string{"reasoning.encrypted_content"},
		"prompt_cache_key":    codexCacheKey(messages),
		"tool_choice":         "auto",
		"parallel_tool_calls": true,
	}

	if len(tools) > 0 {
		body["tools"] = convertToolsForCodex(tools)
	}

	data, err := json.Marshal(body)
	if err != nil {
		return LLMResponse{}, fmt.Errorf("marshal codex request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, codexURL, bytes.NewReader(data))
	if err != nil {
		return LLMResponse{}, fmt.Errorf("build codex request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("chatgpt-account-id", token.AccountID)
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	req.Header.Set("originator", codexOriginator)
	req.Header.Set("User-Agent", "crystaldolphin (go)")
	req.Header.Set("accept", "text/event-stream")
	req.Header.Set("content-type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		s := fmt.Sprintf("Error calling Codex: %v", err)
		return LLMResponse{Content: &s, FinishReason: "error"}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		s := codexFriendlyError(resp.StatusCode, raw)
		return LLMResponse{Content: &s, FinishReason: "error"}, nil
	}

	content, toolCalls, finish, err := consumeCodexSSE(resp.Body)
	if err != nil {
		s := fmt.Sprintf("Error reading Codex SSE: %v", err)
		return LLMResponse{Content: &s, FinishReason: "error"}, nil
	}

	var contentPtr *string
	if content != "" {
		contentPtr = &content
	}
	return LLMResponse{
		Content:      contentPtr,
		ToolCalls:    toolCalls,
		FinishReason: finish,
	}, nil
}

// ---------------------------------------------------------------------------
// SSE consumer
// ---------------------------------------------------------------------------

func consumeCodexSSE(body io.Reader) (string, []ToolCallRequest, string, error) {
	type tcBuf struct {
		id        string
		name      string
		arguments strings.Builder
	}

	var (
		content      strings.Builder
		tcBuffers    = map[string]*tcBuf{}
		toolCalls    []ToolCallRequest
		finishReason = "stop"
	)

	scanner := bufio.NewScanner(body)
	var sseLines []string

	flush := func() {
		defer func() { sseLines = sseLines[:0] }()
		var dataParts []string
		for _, l := range sseLines {
			if strings.HasPrefix(l, "data:") {
				dataParts = append(dataParts, strings.TrimSpace(l[5:]))
			}
		}
		if len(dataParts) == 0 {
			return
		}
		data := strings.Join(dataParts, "\n")
		if data == "[DONE]" || data == "" {
			return
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return
		}

		switch event["type"] {
		case "response.output_item.added":
			item, _ := event["item"].(map[string]any)
			if item["type"] == "function_call" {
				callID, _ := item["call_id"].(string)
				if callID != "" {
					id, _ := item["id"].(string)
					name, _ := item["name"].(string)
					tcBuffers[callID] = &tcBuf{id: id, name: name}
					if args, ok := item["arguments"].(string); ok {
						tcBuffers[callID].arguments.WriteString(args)
					}
				}
			}
		case "response.output_text.delta":
			if delta, ok := event["delta"].(string); ok {
				content.WriteString(delta)
			}
		case "response.function_call_arguments.delta":
			callID, _ := event["call_id"].(string)
			if buf, ok := tcBuffers[callID]; ok {
				if delta, ok := event["delta"].(string); ok {
					buf.arguments.WriteString(delta)
				}
			}
		case "response.function_call_arguments.done":
			callID, _ := event["call_id"].(string)
			if buf, ok := tcBuffers[callID]; ok {
				if args, ok := event["arguments"].(string); ok {
					buf.arguments.Reset()
					buf.arguments.WriteString(args)
				}
			}
		case "response.output_item.done":
			item, _ := event["item"].(map[string]any)
			if item["type"] == "function_call" {
				callID, _ := item["call_id"].(string)
				buf, ok := tcBuffers[callID]
				if !ok {
					break
				}
				argsStr := buf.arguments.String()
				if a, ok := item["arguments"].(string); ok && a != "" {
					argsStr = a
				}
				args, err := repairJSON(argsStr)
				if err != nil {
					args = map[string]any{}
				}
				itemID := buf.id
				if id, ok := item["id"].(string); ok && id != "" {
					itemID = id
				}
				name := buf.name
				if n, ok := item["name"].(string); ok && n != "" {
					name = n
				}
				// Encode both IDs in the tool_call_id so the session stores it for Codex round-trips.
				combinedID := callID
				if itemID != "" {
					combinedID = callID + "|" + itemID
				}
				toolCalls = append(toolCalls, ToolCallRequest{
					ID:        combinedID,
					Name:      name,
					Arguments: args,
				})
			}
		case "response.completed":
			resp, _ := event["response"].(map[string]any)
			status, _ := resp["status"].(string)
			finishReason = codexFinishReason(status)
		case "error", "response.failed":
			return
		}
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flush()
		} else {
			sseLines = append(sseLines, line)
		}
	}
	if len(sseLines) > 0 {
		flush()
	}

	return content.String(), toolCalls, finishReason, scanner.Err()
}

// ---------------------------------------------------------------------------
// Message / tool conversion helpers
// ---------------------------------------------------------------------------

func convertMessagesForCodex(messages MessageHistory) (string, []any) {
	var system string
	var items []any

	for idx, msg := range messages.Messages {
		switch msg.Role {
		case "system":
			if s, ok := msg.Content.(string); ok {
				system = s
			}

		case "user":
			items = append(items, convertCodexUserMessage(msg.Content))

		case "assistant":
			if s, ok := msg.Content.(*string); ok && s != nil && *s != "" {
				items = append(items, map[string]any{
					"type": "message",
					"role": "assistant",
					"content": []any{map[string]any{
						"type": "output_text",
						"text": *s,
					}},
					"status": "completed",
					"id":     fmt.Sprintf("msg_%d", idx),
				})
			} else if s, ok := msg.Content.(string); ok && s != "" {
				items = append(items, map[string]any{
					"type": "message",
					"role": "assistant",
					"content": []any{map[string]any{
						"type": "output_text",
						"text": s,
					}},
					"status": "completed",
					"id":     fmt.Sprintf("msg_%d", idx),
				})
			}
			for _, tc := range msg.ToolCalls {
				callID, itemID := splitCodexToolCallID(tc.ID)
				if callID == "" {
					callID = fmt.Sprintf("call_%d", idx)
				}
				if itemID == "" {
					itemID = fmt.Sprintf("fc_%d", idx)
				}
				argsJSON, _ := json.Marshal(tc.Arguments)
				items = append(items, map[string]any{
					"type":      "function_call",
					"id":        itemID,
					"call_id":   callID,
					"name":      tc.Name,
					"arguments": string(argsJSON),
				})
			}

		case "tool":
			callID, _ := splitCodexToolCallID(msg.ToolCallID)
			if callID == "" {
				callID = "call_0"
			}
			items = append(items, map[string]any{
				"type":    "function_call_output",
				"call_id": callID,
				"output":  anyToString(msg.Content),
			})
		}
	}
	return system, items
}

func convertCodexUserMessage(content any) map[string]any {
	switch c := content.(type) {
	case string:
		return map[string]any{
			"role":    "user",
			"content": []any{map[string]any{"type": "input_text", "text": c}},
		}
	case []any:
		var blocks []any
		for _, item := range c {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			switch m["type"] {
			case "text":
				blocks = append(blocks, map[string]any{"type": "input_text", "text": m["text"]})
			case "image_url":
				if iu, ok := m["image_url"].(map[string]any); ok {
					if url, ok := iu["url"].(string); ok {
						blocks = append(blocks, map[string]any{"type": "input_image", "image_url": url, "detail": "auto"})
					}
				}
			}
		}
		if len(blocks) > 0 {
			return map[string]any{"role": "user", "content": blocks}
		}
	}
	return map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": ""}}}
}

func convertToolsForCodex(tools []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		fn, _ := t["function"].(map[string]any)
		if fn == nil {
			continue
		}
		name, _ := fn["name"].(string)
		if name == "" {
			continue
		}
		params, _ := fn["parameters"].(map[string]any)
		if params == nil {
			params = map[string]any{}
		}
		out = append(out, map[string]any{
			"type":        "function",
			"name":        name,
			"description": fn["description"],
			"parameters":  params,
		})
	}
	return out
}

// ---------------------------------------------------------------------------
// Utilities
// ---------------------------------------------------------------------------

func (p *CodexProvider) loadToken() (*CodexToken, error) {
	data, err := os.ReadFile(p.tokenPath)
	if err != nil {
		return nil, fmt.Errorf("read token file %s: %w", p.tokenPath, err)
	}
	var t CodexToken
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parse token file: %w", err)
	}
	if t.AccessToken == "" {
		return nil, fmt.Errorf("token file has no access_token")
	}
	return &t, nil
}

// SaveCodexToken writes a token to ~/.nanobot/codex_token.json.
// Used by the `provider login openai-codex` command.
func SaveCodexToken(token *CodexToken) error {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".nanobot", "codex_token.json")
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	data, _ := json.MarshalIndent(token, "", "  ")
	return os.WriteFile(path, data, 0o600)
}

func stripCodexPrefix(model string) string {
	for _, pfx := range []string{"openai-codex/", "openai_codex/"} {
		if strings.HasPrefix(model, pfx) {
			return model[len(pfx):]
		}
	}
	return model
}

func splitCodexToolCallID(id any) (callID, itemID string) {
	s, ok := id.(string)
	if !ok || s == "" {
		return "call_0", ""
	}
	if i := strings.Index(s, "|"); i >= 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
}

func codexCacheKey(messages MessageHistory) string {
	// Simple deterministic hash using the JSON representation.
	b, _ := messages.GetHashKey()

	// Use a basic FNV-like approach rather than importing crypto/sha256 just for this.
	h := uint64(14695981039346656037)
	for _, c := range b {
		h ^= uint64(c)
		h *= 1099511628211
	}

	return fmt.Sprintf("%016x", h)
}

var codexFinishReasonMap = map[string]string{
	"completed":  "stop",
	"incomplete": "length",
	"failed":     "error",
	"cancelled":  "error",
}

func codexFinishReason(status string) string {
	if r, ok := codexFinishReasonMap[status]; ok {
		return r
	}
	return "stop"
}

func codexFriendlyError(code int, body []byte) string {
	if code == 429 {
		return "ChatGPT usage quota exceeded or rate limit triggered. Please try again later."
	}
	s := strings.TrimSpace(string(body))
	if len(s) > 300 {
		s = s[:300]
	}
	return fmt.Sprintf("HTTP %d: %s", code, s)
}
