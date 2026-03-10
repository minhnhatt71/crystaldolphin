package channel

import "context"

// Manager is the domain interface for the component that owns all registered
// channels and routes messages between them and the agent.
//
// Implementations live in internal/channels/.
//
//   - Register adds a Channel to the manager's registry.
//   - Start begins listening on all registered channels and dispatches outbound
//     messages; it blocks until ctx is cancelled.
//   - Send routes msg to the channel identified by channelName.
type Manager interface {
	Register(ch Channel)
	Start(ctx context.Context) error
	Send(ctx context.Context, channelName string, msg Message) error
}
