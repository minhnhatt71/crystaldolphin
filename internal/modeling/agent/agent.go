package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/crystaldolphin/crystaldolphin/internal/modeling/llmprovider"
	"github.com/crystaldolphin/crystaldolphin/internal/modeling/prompt"
	"github.com/crystaldolphin/crystaldolphin/internal/shared/llmutils"
)

type Agent struct {
	settings llmprovider.Settings
	provider llmprovider.LLMProvider
	registry ToolHandlerRegistry
}

type AgentChatResult struct {
	Content string
}

func (r *Agent) Chat(ctx context.Context, prompts prompt.Prompts, opts ...ChatOption) (string, error) {
	args := retrieveChatArgs(opts...)

	for i := 0; i < r.settings.MaxIterations(); i++ {
		resp, err := r.provider.Chat(ctx, args.prompt, r.settings)

		if err != nil {
			slog.Error("LLM error", "err", err)
			return "Sorry, I encountered an error calling the LLM.", nil
		}

		if !resp.HasToolCalls() {
			content := ""
			if resp.Content != nil {
				content = *resp.Content
			}

			return llmprovider.StripThink(content), nil
		}

		// Append assistant turn with tool tools.
		tools := llmprovider.NewTools()
		for _, tc := range resp.ToolCalls {
			tools.Add(llmprovider.NewTool(tc.Id(), tc.Name(), tc.Arguments()))
		}

		prompts.Add(prompt.NewAssistant(resp.Content, tools, resp.ReasoningContent))

		// Execute each tool.
		for _, tc := range resp.ToolCalls {
			argsJSON, _ := json.Marshal(tc.Arguments())

			slog.Info("Tool call", "name", tc.Name(), "args", llmutils.Truncate(string(argsJSON), 200))

			var result string
			if t := r.registry.Get(tc.Name()); t != nil {
				result, err = t.Execute(ctx, tc.Arguments())

				if err != nil {
					slog.Error("LLM error", "err", err)
					return "Sorry, I encountered an error calling the LLM.", nil
				}
			} else {
				slog.Error("Tool handler not found", "tool", tc.Name())
				result = fmt.Sprintf("Error: Tool handler for tool '%s' not found", tc.Name())
			}

			prompts.Add(prompt.NewToolCall(tc, &result))
		}
	}

	return "I've reached the maximum number of tool iterations without a final answer.", nil
}
