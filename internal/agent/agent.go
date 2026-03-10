package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	contract "github.com/crystaldolphin/crystaldolphin/internal/modeling/agent"
	"github.com/crystaldolphin/crystaldolphin/internal/modeling/llmprovider"
	"github.com/crystaldolphin/crystaldolphin/internal/modeling/prompt"
	"github.com/crystaldolphin/crystaldolphin/internal/shared/llmutils"
)

// Agent is the concrete implementation of modeling/agent.Agent.
// It runs an LLM ↔ tool iteration loop up to Settings.MaxIterations times.
type Agent struct {
	settings llmprovider.Settings
	provider llmprovider.LLMProvider
	registry contract.ToolHandlerRegistry
}

// New creates a new Agent. The returned value implements modeling/agent.Agent.
func New(settings llmprovider.Settings, provider llmprovider.LLMProvider, registry contract.ToolHandlerRegistry) *Agent {
	return &Agent{
		settings: settings,
		provider: provider,
		registry: registry,
	}
}

// Chat implements modeling/agent.Agent.
func (a *Agent) Chat(ctx context.Context, prompts prompt.Prompts, opts ...contract.ChatOption) (string, error) {
	if args := contract.RetrieveChatArgs(opts...); len(args.Prompt.Get()) > 0 {
		prompts = args.Prompt
	}

	for i := 0; i < a.settings.MaxIterations(); i++ {
		resp, err := a.provider.Chat(ctx, prompts, a.settings)
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

		// Append assistant turn with tool calls.
		tools := llmprovider.NewTools()
		for _, tc := range resp.ToolCalls {
			tools.Add(llmprovider.NewTool(tc.Id(), tc.Name(), tc.Arguments()))
		}
		prompts = prompts.Add(prompt.NewAssistant(resp.Content, tools, resp.ReasoningContent))

		// Execute each tool and append results.
		for _, tc := range resp.ToolCalls {
			argsJSON, _ := json.Marshal(tc.Arguments())
			slog.Info("Tool call", "name", tc.Name(), "args", llmutils.Truncate(string(argsJSON), 200))

			var result string
			if t := a.registry.Get(tc.Name()); t != nil {
				result, err = t.Execute(ctx, tc.Arguments())
				if err != nil {
					slog.Error("Tool error", "tool", tc.Name(), "err", err)
					result = fmt.Sprintf("Error executing tool '%s': %s", tc.Name(), err)
				}
			} else {
				slog.Error("Tool handler not found", "tool", tc.Name())
				result = fmt.Sprintf("Error: Tool handler for tool '%s' not found", tc.Name())
			}

			prompts = prompts.Add(prompt.NewToolCall(tc, &result))
		}
	}

	return "I've reached the maximum number of tool iterations without a final answer.", nil
}
