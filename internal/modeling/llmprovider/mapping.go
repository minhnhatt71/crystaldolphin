package llmprovider

import "github.com/crystaldolphin/crystaldolphin/internal/modeling/prompt"

type Tool struct {
	id        string
	name      string
	arguments map[string]any
}

func NewTool(id, name string, arguments map[string]any) prompt.Tool {
	return &Tool{
		id:        id,
		name:      name,
		arguments: arguments,
	}
}

func (t Tool) Id() string                { return t.id }
func (t Tool) Name() string              { return t.name }
func (t Tool) Arguments() map[string]any { return t.arguments }

type Tools struct {
	tools []prompt.Tool
}

func NewTools(tools ...Tool) prompt.Tools {
	var promptTools []prompt.Tool

	for _, tool := range tools {
		promptTools = append(promptTools, tool)
	}

	return &Tools{tools: promptTools}
}

func (t *Tools) Add(tool prompt.Tool) {
	t.tools = append(t.tools, tool)
}

func (t *Tools) Get(name string) prompt.Tool {
	for _, tool := range t.tools {
		if tool.Name() == name {
			return tool
		}
	}

	return nil
}

func (t *Tools) List() []prompt.Tool {
	var list []prompt.Tool

	for _, tool := range t.tools {
		list = append(list, tool)
	}

	return list
}

type LLMResponse struct {
	Content          *string
	ToolCalls        []Tool
	FinishReason     string
	Usage            map[string]int // "input_tokens", "output_tokens"
	ReasoningContent *string        // DeepSeek-R1 / Kimi thinking block
}

func (r LLMResponse) HasToolCalls() bool { return len(r.ToolCalls) > 0 }
