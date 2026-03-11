package channels

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/crystaldolphin/crystaldolphin/internal/bus"
	modelbus "github.com/crystaldolphin/crystaldolphin/internal/modeling/bus"
	"github.com/crystaldolphin/crystaldolphin/internal/modeling/channel"
	"github.com/crystaldolphin/crystaldolphin/internal/shared/cmdutils"
)

const senderIDCLI = "user"

var cliExitCommands = map[string]bool{
	"exit":  true,
	"quit":  true,
	"/exit": true,
	"/quit": true,
	":q":    true,
}

// CLIChannel wires the terminal (stdin/stdout) into the channel manager so
// that interactive console input reaches the agent via the InboundBus and agent
// replies are printed to stdout via a private console channel.
type CLIChannel struct {
	Base
	console chan modelbus.OutboundMessage
}

// NewCLIChannel creates a CLIChannel.
// inbound is the shared agent inbound bus; outbound is accepted for interface
// compatibility but CLI replies flow through an internal console channel so they
// are not consumed by the manager's dispatchOutbound goroutine.
func NewCLIChannel(inbound *bus.InboundBus, _ *bus.OutboundBus) *CLIChannel {
	return &CLIChannel{
		Base:    NewBase(channel.ChannelCLI, inbound, nil),
		console: make(chan modelbus.OutboundMessage, 8),
	}
}

func (c *CLIChannel) Name() channel.ChannelName { return channel.ChannelCLI }

// Start runs the stdin REPL: reads lines, dispatches them to the agent via the
// inbound bus, and prints each reply received on the outbound bus.
// Blocks until ctx is cancelled or stdin is closed.
func (c *CLIChannel) Start(ctx context.Context) error {
	fmt.Printf("CLI channel ready. Type 'exit' or press Ctrl+C to quit.\n\n")

	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("You: ")

		scanDone := make(chan bool, 1)
		go func() {
			scanDone <- scanner.Scan()
		}()

		select {
		case ok := <-scanDone:
			if !ok {
				fmt.Println("\nGoodbye!")
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if cliExitCommands[strings.ToLower(line)] {
			fmt.Println("Goodbye!")
			return nil
		}

		c.HandleMessage(senderIDCLI, "direct", line, nil, nil)
		c.waitForReply(ctx)
	}
}

// waitForReply blocks until the agent publishes a non-progress reply on the
// outbound bus, then prints it.
func (c *CLIChannel) waitForReply(ctx context.Context) {
	for {
		select {
		case msg := <-c.console:
			if prog, _ := msg.Metadata()["_progress"].(bool); prog {
				fmt.Printf("  ↳ %s\n", msg.Content())
				continue
			}
			cmdutils.PrintResponse(msg.Content())
			return
		case <-ctx.Done():
			return
		}
	}
}

// Send delivers an outbound agent reply to the CLI by publishing it onto the
// outbound bus. The Start loop drains the outbound bus and prints to stdout.
func (c *CLIChannel) Send(_ context.Context, msg channel.Message) error {
	out := modelbus.NewOutboundMessage(channel.ChannelCLI, msg.ChatId(), msg.Content()).
		WithReplyTo(msg.ReplyTo()).
		WithMedia(msg.Media()).
		WithMetadata(msg.Metadata())
	c.console <- out
	return nil
}
