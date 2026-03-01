package tools

import (
	"context"

	"github.com/crystaldolphin/crystaldolphin/internal/bus"
)

// TurnContext carries per-turn routing metadata through the context tree.
// It is set by the agent loop once per message and read by stateful tools
// (message, spawn, cron) inside Execute
type TurnContext struct {
	Channel bus.Channel
	ChatID  string
	MsgID   string

	// MessageSent is closed by MessageTool.Execute when it delivers a message.
	// The agent loop checks it after runLoop via a non-blocking receive to
	// decide whether to suppress the automatic reply.
	MessageSent chan struct{}
}

type turnKey struct{}

// WithTurn returns a child context that carries tc.
func WithTurn(ctx context.Context, tc TurnContext) context.Context {
	return context.WithValue(ctx, turnKey{}, tc)
}

// TurnCtx extracts the TurnContext from ctx.
// Returns a zero-value TurnContext if none was set.
func TurnCtx(ctx context.Context) TurnContext {
	tc, _ := ctx.Value(turnKey{}).(TurnContext)
	return tc
}
