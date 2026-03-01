package bus

// ConsoleBus carries messages from the agent → CLI REPL.
// It is separate from ChannelBus so CLI output is not drained by
// the channel manager's dispatchOutbound goroutine.
type ConsoleBus struct {
	ch chan ChannelMessage
}

func NewConsoleBus(bufSize int) *ConsoleBus {
	return &ConsoleBus{ch: make(chan ChannelMessage, bufSize)}
}

// Publish delivers a reply to the CLI REPL.
func (b *ConsoleBus) Publish(msg ChannelMessage) {
	b.ch <- msg
}

// Subscribe returns a receive-only view of the console channel.
func (b *ConsoleBus) Subscribe() <-chan ChannelMessage {
	return b.ch
}
