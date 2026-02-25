package tools

import "encoding/json"

// ToolList is a pre-built slice of tool definitions in OpenAI function-calling
// format. Build one from a set of tools via NewToolList(), then pass it
// directly to LLM provider calls.
type ToolList []map[string]any

// NewToolList converts a collection of Tools into a ToolList.
func NewToolList(tools map[string]Tool) ToolList {
	list := make(ToolList, 0, len(tools))
	for _, t := range tools {
		var params any
		if err := json.Unmarshal(t.Parameters(), &params); err != nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		list = append(list, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name(),
				"description": t.Description(),
				"parameters":  params,
			},
		})
	}
	return list
}
