package agent

import "context"

type ToolHandler interface {
	Execute(ctx context.Context, args map[string]any) (string, error)
}

type ToolHandlerRegistry interface {
	Get(name string) ToolHandler
}
