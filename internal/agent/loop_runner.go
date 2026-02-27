package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/crystaldolphin/crystaldolphin/internal/schema"
	"github.com/crystaldolphin/crystaldolphin/internal/shared/llmutils"
	"github.com/crystaldolphin/crystaldolphin/internal/tools"
)

// LoopRunner executes the LLM ↔ tool iteration loop.
// It is embedded by CoreAgent and SubAgent to share the loop body.
type LoopRunner struct {
	provider schema.LLMProvider
	settings schema.AgentSettings
}

func newLoopRunner(provider schema.LLMProvider, settings schema.AgentSettings) LoopRunner {
	return LoopRunner{provider: provider, settings: settings}
}

// run is the canonical LLM ↔ tool loop body shared by CoreAgent and SubAgent.
// tls is passed by pointer so CoreAgent can share AgentLoop.tools (MCP-extended live map).
func (r *LoopRunner) run(ctx context.Context, conversation schema.Messages, tls *tools.ToolList, onProgress func(string)) (finalContent string, toolsUsed []string) {
	for i := 0; i < r.settings.MaxIter; i++ {
		resp, err := r.provider.Chat(ctx,
			conversation,
			tls.Definitions(),
			schema.NewChatOptions(r.settings.Model, r.settings.MaxTokens, r.settings.Temperature),
		)

		if err != nil {
			slog.Error("LLM error", "err", err)
			return "Sorry, I encountered an error calling the LLM.", nil
		}

		if len(resp.ToolCalls) == 0 {
			// Terminal response.
			content := ""
			if resp.Content != nil {
				content = *resp.Content
			}
			return llmutils.StripThink(content), toolsUsed
		}

		// Progress: emit partial text + tool hint.
		if onProgress != nil {
			if resp.Content != nil {
				if clean := llmutils.StripThink(*resp.Content); clean != "" {
					onProgress(clean)
				}
			}
			onProgress(llmutils.ToolHint(resp.ToolCalls))
		}

		// Append assistant turn with tool calls.
		var toolCalls []schema.ToolCall
		for _, tc := range resp.ToolCalls {
			toolCalls = append(toolCalls, schema.ToolCall{ID: tc.Id, Name: tc.Name, Arguments: tc.Arguments})
		}

		conversation.AddAssistant(resp.Content, toolCalls, resp.ReasoningContent)

		// Execute each tool.
		for _, tc := range resp.ToolCalls {
			toolsUsed = append(toolsUsed, tc.Name)
			argsJSON, _ := json.Marshal(tc.Arguments)

			slog.Info("Tool call", "name", tc.Name, "args", llmutils.Truncate(string(argsJSON), 200))

			var result string
			if t := tls.Get(tc.Name); t != nil {
				result, _ = t.Execute(ctx, tc.Arguments)
			} else {
				result = fmt.Sprintf("Error: Tool '%s' not found", tc.Name)
			}

			conversation.AddToolResult(tc.Id, tc.Name, result)
		}
	}

	return "I've reached the maximum number of tool iterations without a final answer.", toolsUsed
}
