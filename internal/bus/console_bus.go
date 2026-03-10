package bus

import "github.com/crystaldolphin/crystaldolphin/internal/modeling/channel"

// ConsoleBus carries messages from the agent → CLI REPL.
// It is separate from ChannelBus so CLI output is not drained by
// the channel manager's dispatchOutbound goroutine.
type ConsoleBus struct {
	ch chan channel.Message
}

func NewConsoleBus(bufSize int) *ConsoleBus {
	return &ConsoleBus{ch: make(chan channel.Message, bufSize)}
}

// Publish delivers a reply to the CLI REPL.
func (b *ConsoleBus) Publish(msg channel.Message) {
	b.ch <- msg
}

// Subscribe returns a receive-only view of the console channel.
func (b *ConsoleBus) Subscribe() <-chan channel.Message {
	return b.ch
}
