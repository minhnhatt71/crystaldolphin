package tools

import (
	"context"
	"sync/atomic"

	"github.com/crystaldolphin/crystaldolphin/internal/buslegacy"
)

// turnContext carries per-turn routing metadata through the context tree.
// It is set by the agent loop once per message and read by stateful tools
// (message, spawn, cron) inside Execute
type turnContext struct {
	channel   buslegacy.Channel
	chatId    string
	messageId string
	published *atomic.Bool
}

type turnKey struct{}

// WithTurn returns a child context that carries tc.
func WithTurn(ctx context.Context, channel buslegacy.Channel, chatId, msgID string) context.Context {
	return context.WithValue(ctx, turnKey{}, &turnContext{
		channel:   channel,
		chatId:    chatId,
		messageId: msgID,
		published: &atomic.Bool{},
	})
}

// TurnCtx extracts the TurnContext from ctx.
// Returns a zero-value TurnContext if none was set.
func TurnCtx(ctx context.Context) turnContext {
	if tc, ok := ctx.Value(turnKey{}).(*turnContext); ok && tc != nil {
		return *tc
	}
	return turnContext{}
}

func (ctx turnContext) PublishedToChannels() bool {
	if ctx.published == nil {
		return false
	}
	return ctx.published.Load()
}
