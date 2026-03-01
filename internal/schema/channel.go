package schema

import (
	"context"

	"github.com/crystaldolphin/crystaldolphin/internal/bus"
)

// Channel is the interface every chat-platform adapter must implement.
type Channel interface {
	// Name returns the unique channel identifier (e.g. "telegram").
	Name() string
	// Start begins listening for incoming messages; it blocks until ctx is cancelled.
	Start(ctx context.Context) error
	// Send delivers an outbound message to the platform.
	Send(ctx context.Context, msg bus.ChannelMessage) error
}
