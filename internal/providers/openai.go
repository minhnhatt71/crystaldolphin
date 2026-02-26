package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/crystaldolphin/crystaldolphin/internal/schema"
)

// OpenAIProvider makes direct HTTP calls to any OpenAI-compatible endpoint,
// and also handles the Anthropic Messages API as a special case.
type OpenAIProvider struct {
	apiKey       string
	apiBase      string
	defaultModel string
	extraHeaders map[string]string
	gateway      *ProviderSpec // non-nil for gateway/local providers
	spec         *ProviderSpec // non-nil for standard providers
	isAnthropic  bool
	httpClient   *http.Client
}

// NewOpenAIProvider constructs a provider from raw config values.
// The caller extracts these from config.Config to avoid an import cycle.
func NewOpenAIProvider(
	apiKey, apiBase, defaultModel, providerName string,
	extraHeaders map[string]string,
) *OpenAIProvider {
	gateway := FindGateway(providerName, apiKey, apiBase)

	var spec *ProviderSpec
	if gateway == nil {
		spec = FindByModel(defaultModel)
		if spec == nil {
			spec = FindByName(providerName)
		}
	}

	// Resolve effective API base.
	effectiveBase := apiBase
	if effectiveBase == "" {
		if gateway != nil && gateway.DefaultAPIBase != "" {
			effectiveBase = gateway.DefaultAPIBase
		} else if spec != nil && spec.DefaultAPIBase != "" {
			effectiveBase = spec.DefaultAPIBase
		} else {
			effectiveBase = "https://api.openai.com/v1"
		}
	}
	effectiveBase = strings.TrimRight(effectiveBase, "/")

	isAnthropic := providerName == "anthropic" ||
		strings.Contains(strings.ToLower(effectiveBase), "anthropic.com")

	return &OpenAIProvider{
		apiKey:       apiKey,
		apiBase:      effectiveBase,
		defaultModel: defaultModel,
		extraHeaders: extraHeaders,
		gateway:      gateway,
		spec:         spec,
		isAnthropic:  isAnthropic,
		httpClient:   &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *OpenAIProvider) DefaultModel() string { return p.defaultModel }

// Chat implements schema.LLMProvider. It dispatches to Anthropic or OpenAI-compat paths.
func (p *OpenAIProvider) Chat(
	ctx context.Context,
	messages schema.Messages,
	tools []map[string]any,
	opts schema.ChatOptions,
) (schema.LLMResponse, error) {
	model := opts.Model
	if model == "" {
		model = p.defaultModel
	}
	origModel := model

	// Prompt caching (Anthropic + OpenRouter).
	if p.supportsPromptCaching(origModel) {
		messages, tools = applyCacheControl(messages, tools)
	}

	maxTokens := opts.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	if p.isAnthropic {
		return p.chatAnthropic(ctx, messages, tools, p.resolveModel(model), maxTokens, opts.Temperature)
	}

	return p.chatOpenAI(ctx, messages, tools, p.resolveModel(model), maxTokens, opts.Temperature)
}

// ---------------------------------------------------------------------------
// OpenAI-compatible path
// ---------------------------------------------------------------------------

func (p *OpenAIProvider) chatOpenAI(
	ctx context.Context,
	messages schema.Messages,
	tools []map[string]any,
	model string,
	maxTokens int,
	temperature float64,
) (schema.LLMResponse, error) {
	body := map[string]any{
		"model":       model,
		"messages":    sanitizeMessages(messages),
		"max_tokens":  maxTokens,
		"temperature": temperature,
	}
	if len(tools) > 0 {
		body["tools"] = tools
		body["tool_choice"] = "auto"
	}
	p.applyModelOverrides(model, body)

	data, err := json.Marshal(body)
	if err != nil {
		return schema.LLMResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.apiBase+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return schema.LLMResponse{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	for k, v := range p.extraHeaders {
		req.Header.Set(k, v)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return schema.LLMResponse{}, fmt.Errorf("HTTP request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return schema.LLMResponse{}, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return errResponse(fmt.Sprintf("HTTP %d: %s", resp.StatusCode, friendlyHTTPError(resp.StatusCode, raw)))
	}

	return parseOpenAIResponse(raw)
}

// ---------------------------------------------------------------------------
// Anthropic Messages API path
// ---------------------------------------------------------------------------

func (p *OpenAIProvider) chatAnthropic(
	ctx context.Context,
	messages schema.Messages,
	tools []map[string]any,
	model string,
	maxTokens int,
	temperature float64,
) (schema.LLMResponse, error) {
	system, converted := convertMessagesToAnthropic(messages)

	body := map[string]any{
		"model":       model,
		"messages":    converted,
		"max_tokens":  maxTokens,
		"temperature": temperature,
	}
	if system != "" {
		body["system"] = system
	}
	if len(tools) > 0 {
		body["tools"] = convertToolsToAnthropic(tools)
	}

	data, err := json.Marshal(body)
	if err != nil {
		return schema.LLMResponse{}, fmt.Errorf("marshal anthropic request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.apiBase+"/messages", bytes.NewReader(data))
	if err != nil {
		return schema.LLMResponse{}, fmt.Errorf("build anthropic request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	for k, v := range p.extraHeaders {
		req.Header.Set(k, v)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return schema.LLMResponse{}, fmt.Errorf("anthropic HTTP request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return schema.LLMResponse{}, fmt.Errorf("read anthropic response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return errResponse(fmt.Sprintf("HTTP %d: %s", resp.StatusCode, friendlyHTTPError(resp.StatusCode, raw)))
	}

	return parseAnthropicResponse(raw)
}

// ---------------------------------------------------------------------------
// Model resolution
// ---------------------------------------------------------------------------

// resolveModel strips routing prefixes from the model string so the provider
// API receives the bare model name it expects.
//
// Gateway providers (e.g. OpenRouter) keep the "provider/model" sub-prefix
// because the gateway needs it for routing. The gateway's own LiteLLM prefix
// ("openrouter/") is stripped if present. AiHubMix (strip_model_prefix=true)
// strips everything down to the bare model name.
//
// Standard providers (Anthropic, DeepSeek, etc.) strip any provider-name
// prefix so the API receives just the model name.
func (p *OpenAIProvider) resolveModel(model string) string {
	if p.gateway != nil {
		if p.gateway.StripModelPrefix {
			if i := strings.LastIndex(model, "/"); i >= 0 {
				return model[i+1:]
			}
			return model
		}
		// Strip only the gateway's own litellm prefix if present.
		if pfx := p.gateway.LiteLLMPrefix; pfx != "" {
			full := pfx + "/"
			if strings.HasPrefix(strings.ToLower(model), full) {
				model = model[len(full):]
			}
		}
		return model
	}

	// Standard/local provider: strip known provider-name prefix.
	prefixesToStrip := []string{}
	if p.spec != nil {
		prefixesToStrip = append(prefixesToStrip, p.spec.LiteLLMPrefix, p.spec.Name)
	}
	for _, pfx := range prefixesToStrip {
		if pfx == "" {
			continue
		}
		full := pfx + "/"
		if strings.HasPrefix(strings.ToLower(model), full) {
			return model[len(full):]
		}
	}
	// Fallback: strip any unknown provider prefix recognised in registry.
	if strings.Contains(model, "/") {
		parts := strings.SplitN(model, "/", 2)
		norm := strings.ReplaceAll(strings.ToLower(parts[0]), "-", "_")
		if FindByName(norm) != nil {
			return parts[1]
		}
	}
	return model
}

// ---------------------------------------------------------------------------
// Prompt caching
// ---------------------------------------------------------------------------

func (p *OpenAIProvider) supportsPromptCaching(model string) bool {
	if p.gateway != nil {
		return p.gateway.SupportsPromptCaching
	}
	spec := FindByModel(model)
	return spec != nil && spec.SupportsPromptCaching
}

// applyCacheControl injects cache_control ephemeral blocks on the last system
// message content block and the last tool definition.
func applyCacheControl(messages schema.Messages, tools []map[string]any) (schema.Messages, []map[string]any) {
	out := schema.NewMessages()
	out.Messages = make([]schema.Message, len(messages.Messages))
	for i, msg := range messages.Messages {
		if msg.Role == "system" {
			newMsg := msg
			switch c := msg.Content.(type) {
			case string:
				newMsg.Content = []any{
					map[string]any{"type": "text", "text": c, "cache_control": map[string]any{"type": "ephemeral"}},
				}
			case []any:
				arr := make([]any, len(c))
				copy(arr, c)
				if len(arr) > 0 {
					if m, ok := arr[len(arr)-1].(map[string]any); ok {
						last := copyAnyMap(m)
						last["cache_control"] = map[string]any{"type": "ephemeral"}
						arr[len(arr)-1] = last
					}
				}
				newMsg.Content = arr
			}
			out.Messages[i] = newMsg
		} else {
			out.Messages[i] = msg
		}
	}

	if len(tools) == 0 {
		return out, tools
	}
	newTools := make([]map[string]any, len(tools))
	copy(newTools, tools)
	last := copyMap(newTools[len(newTools)-1])
	last["cache_control"] = map[string]any{"type": "ephemeral"}
	newTools[len(newTools)-1] = last
	return out, newTools
}

// ---------------------------------------------------------------------------
// Message sanitisation
// ---------------------------------------------------------------------------

// messageToWireMap converts a typed Message to the OpenAI wire-format map.
func messageToWireMap(m schema.Message) map[string]any {
	wire := map[string]any{
		"role":    m.Role,
		"content": m.Content,
	}
	if m.Role == "assistant" {
		// Strict providers require "content" even for tool-call-only messages.
		if m.Content == nil {
			wire["content"] = nil
		}
		if len(m.ToolCalls) > 0 {
			raw := make([]map[string]any, len(m.ToolCalls))
			for i, tc := range m.ToolCalls {
				raw[i] = tc.ToWireMap()
			}
			wire["tool_calls"] = raw
		}
		if m.ReasoningContent != nil {
			wire["reasoning_content"] = *m.ReasoningContent
		}
	}
	if m.Role == "tool" {
		wire["tool_call_id"] = m.ToolCallID
		wire["name"] = m.ToolName
	}
	return wire
}

func sanitizeMessages(messages schema.Messages) []map[string]any {
	out := make([]map[string]any, 0, len(messages.Messages))
	for _, m := range messages.Messages {
		out = append(out, messageToWireMap(m))
	}
	return out
}

// ---------------------------------------------------------------------------
// Model overrides
// ---------------------------------------------------------------------------

func (p *OpenAIProvider) applyModelOverrides(model string, body map[string]any) {
	modelLower := strings.ToLower(model)
	var spec *ProviderSpec
	if p.spec != nil {
		spec = p.spec
	} else {
		spec = FindByModel(model)
	}
	if spec == nil {
		return
	}
	for _, ov := range spec.ModelOverrides {
		if strings.Contains(modelLower, strings.ToLower(ov.Pattern)) {
			for k, v := range ov.Overrides {
				body[k] = v
			}
			return
		}
	}
}

// ---------------------------------------------------------------------------
// Anthropic format helpers
// ---------------------------------------------------------------------------

// convertMessagesToAnthropic converts typed messages to Anthropic's wire format.
// Returns (system_prompt, converted_messages).
func convertMessagesToAnthropic(messages schema.Messages) (string, []map[string]any) {
	var system string
	var out []map[string]any

	for _, msg := range messages.Messages {
		switch msg.Role {
		case "system":
			if s, ok := msg.Content.(string); ok {
				if system != "" {
					system += "\n\n"
				}
				system += s
			}

		case "user":
			out = append(out, map[string]any{
				"role":    "user",
				"content": normalizeContentForAnthropic(msg.Content),
			})

		case "tool":
			block := map[string]any{
				"type":        "tool_result",
				"tool_use_id": msg.ToolCallID,
				"content":     anyToString(msg.Content),
			}
			// Merge consecutive tool results into one user message.
			if len(out) > 0 && out[len(out)-1]["role"] == "user" {
				prev := out[len(out)-1]
				switch c := prev["content"].(type) {
				case []any:
					prev["content"] = append(c, block)
				default:
					prev["content"] = []any{block}
				}
			} else {
				out = append(out, map[string]any{"role": "user", "content": []any{block}})
			}

		case "assistant":
			var blocks []any
			if s, ok := msg.Content.(*string); ok && s != nil && *s != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": *s})
			} else if s, ok := msg.Content.(string); ok && s != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": s})
			}
			for _, tc := range msg.ToolCalls {
				blocks = append(blocks, map[string]any{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Name,
					"input": tc.Arguments,
				})
			}
			if len(blocks) == 0 {
				blocks = []any{map[string]any{"type": "text", "text": ""}}
			}
			out = append(out, map[string]any{"role": "assistant", "content": blocks})
		}
	}
	return system, out
}

// convertToolsToAnthropic converts OpenAI function schemas to Anthropic tool format.
// Key difference: "parameters" → "input_schema".
func convertToolsToAnthropic(tools []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		fn, _ := t["function"].(map[string]any)
		if fn == nil {
			continue
		}
		at := map[string]any{
			"name":         fn["name"],
			"description":  fn["description"],
			"input_schema": fn["parameters"],
		}
		// Forward cache_control if present (prompt caching).
		if cc, ok := t["cache_control"]; ok {
			at["cache_control"] = cc
		}
		out = append(out, at)
	}
	return out
}

func normalizeContentForAnthropic(content any) any {
	if content == nil {
		return []any{map[string]any{"type": "input_text", "text": ""}}
	}
	if s, ok := content.(string); ok {
		return s // Anthropic accepts plain string for user messages
	}
	return content
}

// ---------------------------------------------------------------------------
// Response parsers
// ---------------------------------------------------------------------------

// openAIRespBody is the subset of the OpenAI chat completion response we care about.
type openAIRespBody struct {
	Choices []struct {
		Message struct {
			Content          any `json:"content"`
			ReasoningContent any `json:"reasoning_content"`
			ToolCalls        []struct {
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func parseOpenAIResponse(raw []byte) (schema.LLMResponse, error) {
	var body openAIRespBody
	if err := json.Unmarshal(raw, &body); err != nil {
		return schema.LLMResponse{}, fmt.Errorf("parse OpenAI response: %w", err)
	}
	if len(body.Choices) == 0 {
		return schema.LLMResponse{}, fmt.Errorf("empty choices in response")
	}

	msg := body.Choices[0].Message

	var content *string
	switch c := msg.Content.(type) {
	case string:
		if c != "" {
			content = &c
		}
	}

	var reasoningContent *string
	switch r := msg.ReasoningContent.(type) {
	case string:
		if r != "" {
			reasoningContent = &r
		}
	}

	var toolCalls []schema.ToolCallRequest
	for _, tc := range msg.ToolCalls {
		args, err := repairJSON(tc.Function.Arguments)
		if err != nil {
			slog.Warn("failed to parse tool arguments", "tool", tc.Function.Name, "err", err)
			args = map[string]any{}
		}
		toolCalls = append(toolCalls, schema.ToolCallRequest{
			Id:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: args,
		})
	}

	usage := map[string]int{
		"prompt_tokens":     body.Usage.PromptTokens,
		"completion_tokens": body.Usage.CompletionTokens,
		"total_tokens":      body.Usage.TotalTokens,
	}

	finish := body.Choices[0].FinishReason
	if finish == "" {
		finish = "stop"
	}

	return schema.LLMResponse{
		Content:          content,
		ToolCalls:        toolCalls,
		FinishReason:     finish,
		Usage:            usage,
		ReasoningContent: reasoningContent,
	}, nil
}

// anthropicRespBody models the Anthropic Messages API response.
type anthropicRespBody struct {
	Content []struct {
		Type  string         `json:"type"`
		Text  string         `json:"text"`  // type=text
		ID    string         `json:"id"`    // type=tool_use
		Name  string         `json:"name"`  // type=tool_use
		Input map[string]any `json:"input"` // type=tool_use
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func parseAnthropicResponse(raw []byte) (schema.LLMResponse, error) {
	var body anthropicRespBody
	if err := json.Unmarshal(raw, &body); err != nil {
		return schema.LLMResponse{}, fmt.Errorf("parse Anthropic response: %w", err)
	}

	var contentStr string
	var toolCalls []schema.ToolCallRequest

	for _, block := range body.Content {
		switch block.Type {
		case "text":
			contentStr += block.Text
		case "tool_use":
			toolCalls = append(toolCalls, schema.ToolCallRequest{
				Id:        block.ID,
				Name:      block.Name,
				Arguments: block.Input,
			})
		}
	}

	var content *string
	if contentStr != "" {
		content = &contentStr
	}

	finish := "stop"
	if body.StopReason == "tool_use" {
		finish = "tool_calls"
	} else if body.StopReason != "" && body.StopReason != "end_turn" {
		finish = body.StopReason
	}

	usage := map[string]int{
		"prompt_tokens":     body.Usage.InputTokens,
		"completion_tokens": body.Usage.OutputTokens,
		"total_tokens":      body.Usage.InputTokens + body.Usage.OutputTokens,
	}

	return schema.LLMResponse{
		Content:      content,
		ToolCalls:    toolCalls,
		FinishReason: finish,
		Usage:        usage,
	}, nil
}

// ---------------------------------------------------------------------------
// JSON repair
// ---------------------------------------------------------------------------

// repairJSON attempts to unmarshal JSON, retrying after stripping trailing
// garbage characters. This handles some LLMs that emit truncated tool arguments.
func repairJSON(raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{}, nil
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err == nil {
		return out, nil
	}

	// Attempt 1: trim trailing non-JSON characters.
	stripped := strings.TrimRight(raw, " \t\n\r}]")
	if !strings.HasSuffix(stripped, "}") {
		stripped += "}"
	}
	if err := json.Unmarshal([]byte(stripped), &out); err == nil {
		return out, nil
	}

	// Attempt 2: find the last complete JSON object.
	if i := strings.LastIndex(raw, "}"); i >= 0 {
		if err := json.Unmarshal([]byte(raw[:i+1]), &out); err == nil {
			return out, nil
		}
	}

	return map[string]any{}, fmt.Errorf("cannot repair JSON: %s", raw)
}

// ---------------------------------------------------------------------------
// Utilities
// ---------------------------------------------------------------------------

func errResponse(msg string) (schema.LLMResponse, error) {
	s := msg
	return schema.LLMResponse{Content: &s, FinishReason: "error"}, nil
}

func friendlyHTTPError(code int, body []byte) string {
	if code == 429 {
		return "rate limit exceeded"
	}
	s := strings.TrimSpace(string(body))
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}

func copyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func copyAnyMap(m map[string]any) map[string]any { return copyMap(m) }

func anyToString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}
