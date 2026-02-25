package tools

import "context"

// TurnContext carries per-turn routing metadata through the context tree.
// It is set by the agent loop once per message and read by stateful tools
// (message, spawn, cron) inside Execute — eliminating the need for mutable
// SetContext calls on each tool singleton.
type TurnContext struct {
	Channel string
	ChatID  string
	MsgID   string

	// MessageSent is flipped to true by MessageTool.Execute when it delivers
	// a message.  The agent loop reads it after runLoop to decide whether to
	// suppress the automatic reply.  Using a pointer so the flag is shared
	// between the context value and the caller that holds the original.
	MessageSent *bool
}

type turnKey struct{}

// WithTurnContext returns a child context that carries tc.
func WithTurnContext(ctx context.Context, tc TurnContext) context.Context {
	return context.WithValue(ctx, turnKey{}, tc)
}

// TurnCtx extracts the TurnContext from ctx.
// Returns a zero-value TurnContext if none was set.
func TurnCtx(ctx context.Context) TurnContext {
	tc, _ := ctx.Value(turnKey{}).(TurnContext)
	return tc
}
