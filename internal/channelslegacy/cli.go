package channelslegacy

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/crystaldolphin/crystaldolphin/internal/buslegacy"
	channelmodel "github.com/crystaldolphin/crystaldolphin/internal/modeling/channel"
	"github.com/crystaldolphin/crystaldolphin/internal/shared/cmdutils"
)

var cliExitCommands = map[string]bool{
	"exit":  true,
	"quit":  true,
	"/exit": true,
	"/quit": true,
	":q":    true,
}

// CLIChannel wires the terminal (stdin/stdout) into the channel manager so
// that interactive console input reaches the agent via the AgentBus and agent
// replies are printed to stdout via the ConsoleBus.
type CLIChannel struct {
	Base
	console *buslegacy.ConsoleBus
}

// NewCLIChannel creates a CLIChannel.
func NewCLIChannel(inbound *buslegacy.AgentBus, console *buslegacy.ConsoleBus) *CLIChannel {
	return &CLIChannel{
		Base:    NewBase(buslegacy.ChannelCLI, inbound, nil),
		console: console,
	}
}

func (c *CLIChannel) Name() string { return string(buslegacy.ChannelCLI) }

// Start runs the stdin REPL: reads lines, dispatches them to the agent via the
// inbound bus, and prints each reply received on the console bus.
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

		c.HandleMessage(buslegacy.SenderIdCLI, "direct", line, nil, nil)
		c.waitForReply(ctx)
	}
}

// waitForReply blocks until the agent publishes a non-progress reply on the
// console bus, then prints it.
func (c *CLIChannel) waitForReply(ctx context.Context) {
	for {
		select {
		case msg := <-c.console.Subscribe():
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
// console bus. The Start loop drains the console bus and prints to stdout.
func (c *CLIChannel) Send(_ context.Context, msg buslegacy.ChannelMessage) error {
	c.console.Publish(
		channelmodel.NewMessage(msg.ChatId(), msg.Content()).
			WithReplyTo(msg.ReplyTo()).
			WithMedia(msg.Media()).
			WithMetadata(msg.Metadata()),
	)
	return nil
}
