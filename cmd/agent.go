package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/crystaldolphin/crystaldolphin/internal/bus"
	"github.com/crystaldolphin/crystaldolphin/internal/config"
	"github.com/crystaldolphin/crystaldolphin/internal/dependency"
	"github.com/crystaldolphin/crystaldolphin/internal/schema"
)

var (
	message  string
	key      string
	markdown bool
	logs     bool
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Interact with the agent",
	RunE:  runAgent,
}

func init() {
	agentCmd.Flags().StringVarP(&message, "message", "m", "", "Send a single message and exit")
	agentCmd.Flags().StringVarP(&key, "key", "s", "cli:direct", "Routing key")
	agentCmd.Flags().BoolVar(&markdown, "markdown", true, "Render output as Markdown (no-op: plain output)")
	agentCmd.Flags().BoolVar(&logs, "logs", false, "Show runtime logs")
}

var exitCommands = map[string]bool{
	"exit":  true,
	"quit":  true,
	"/exit": true,
	"/quit": true,
	":q":    true,
}

func runAgent(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load(config.ConfigPath())
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	container, err := dependency.New(cfg)
	if err != nil {
		return err
	}

	routingKey := key
	channel, chatId := parseRoutingKey(routingKey)

	loop := container.AgentLoop()
	messageBus := container.MessageBus()

	if message != "" {
		return runSingleMessage(loop, routingKey, channel, chatId)
	}

	return runInteractive(loop, messageBus, channel, chatId)
}

// runSingleMessage sends one message to the agent and prints the response.
func runSingleMessage(loop schema.AgentLooper, key string, channel bus.ChannelType, chatId string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	msg := bus.NewInboundMessage(channel, "user", chatId, message, key)

	fmt.Fprintf(os.Stderr, "  ↳ thinking...\n")
	res := loop.ProcessDirect(ctx, msg)

	printResponse(res)
	return nil
}

// runInteractive starts the REPL loop: reads lines from stdin, sends each to
// the agent via the bus, and waits for each reply before prompting again.
func runInteractive(loop schema.AgentLooper, msgBus bus.Bus, channel bus.ChannelType, chatId string) error {
	fmt.Printf("%s Interactive mode (type 'exit' or Ctrl+C to quit)\n\n", logo)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listenForSignals(cancel)

	go func() { _ = loop.Run(ctx) }()

	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("You: ")

		if !scanner.Scan() {
			fmt.Println("\nGoodbye!")
			return nil
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if exitCommands[strings.ToLower(line)] {
			fmt.Println("Goodbye!")
			return nil
		}

		sendAndWait(ctx, msgBus, channel, chatId, line)
	}
}

// listenForSignals cancels ctx on SIGINT or SIGTERM and exits.
func listenForSignals(cancel context.CancelFunc) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		switch <-sigChan {
		case syscall.SIGINT:
			fmt.Println("\nGoodbye!")
			fmt.Println("\nReceived SIGINT, shutting down...")
		case syscall.SIGTERM:
			fmt.Println("\nGoodbye!")
			fmt.Println("\nReceived SIGTERM, shutting down...")
		}

		cancel()
		os.Exit(0)
	}()
}

// sendAndWait pushes a message onto the inbound bus and blocks until the agent
// publishes the final reply (or ctx is cancelled).
func sendAndWait(ctx context.Context, msgBus bus.Bus, channel bus.ChannelType, chatId, content string) {
	msgBus.PublishInbound(bus.NewInboundMessage(channel, "user", chatId, content, ""))

	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		for {
			select {
			case msg := <-msgBus.SubscribeOutbound():
				if prog, _ := msg.Metadata()["_progress"].(bool); prog {
					fmt.Printf("  ↳ %s\n", msg.Content())
					continue
				}
				if msg.Content() != "" {
					printResponse(msg.Content())
				}
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	<-doneCh
}

func printResponse(text string) {
	fmt.Printf("\n%s crystaldolphin\n%s\n\n", logo, text)
}

func parseRoutingKey(key string) (channel bus.ChannelType, chatID string) {
	if i := strings.Index(key, ":"); i >= 0 {
		return bus.ChannelType(key[:i]), key[i+1:]
	}
	return bus.ChannelCLI, key
}
