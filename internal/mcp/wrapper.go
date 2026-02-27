package mcp

import (
	"context"
	"encoding/json"

	"github.com/crystaldolphin/crystaldolphin/internal/schema"
)

// toolWrapper wraps a single tool discovered from an MCP server and implements schema.Tool.
type toolWrapper struct {
	client      *client
	name        string
	origName    string
	description string
	parameters  json.RawMessage
}

func (w *toolWrapper) Name() string                { return w.name }
func (w *toolWrapper) Description() string         { return w.description }
func (w *toolWrapper) Parameters() json.RawMessage { return w.parameters }

func (w *toolWrapper) Execute(ctx context.Context, params map[string]any) (string, error) {
	return w.client.callTool(ctx, w.origName, params)
}

// Ensure toolWrapper implements schema.Tool at compile time.
var _ schema.Tool = (*toolWrapper)(nil)
