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
	agentMessage  string
	agentSession  string
	agentMarkdown bool
	agentLogs     bool
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Interact with the agent",
	RunE:  runAgent,
}

func init() {
	agentCmd.Flags().StringVarP(&agentMessage, "message", "m", "", "Send a single message and exit")
	agentCmd.Flags().StringVarP(&agentSession, "session", "s", "cli:direct", "Session ID")
	agentCmd.Flags().BoolVar(&agentMarkdown, "markdown", true, "Render output as Markdown (no-op: plain output)")
	agentCmd.Flags().BoolVar(&agentLogs, "logs", false, "Show runtime logs")
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

	sessionKey := agentSession
	channel, chatId := parseSessionKey(sessionKey)

	loop := container.AgentLoop()
	messageBus := container.MessageBus()

	if agentMessage != "" {
		return runSingleMessage(loop, sessionKey, channel, chatId)
	}

	return runInteractive(loop, messageBus, channel, chatId)
}

// runSingleMessage sends one message to the agent and prints the response.
func runSingleMessage(loop schema.AgentLooper, sessionKey, channel, chatId string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	fmt.Fprintf(os.Stderr, "  ↳ thinking...\n")
	res := loop.ProcessDirect(ctx, agentMessage, sessionKey, channel, chatId)

	printResponse(res)
	return nil
}

// runInteractive starts the REPL loop: reads lines from stdin, sends each to
// the agent via the bus, and waits for each reply before prompting again.
func runInteractive(loop schema.AgentLooper, msgBus bus.Bus, channel, chatId string) error {
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
func sendAndWait(ctx context.Context, msgBus bus.Bus, channel, chatId, content string) {
	msgBus.PublishInbound(bus.NewInboundMessage(channel, "user", chatId, content))

	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		for {
			select {
			case msg := <-msgBus.OutboundChan():
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

func parseSessionKey(key string) (channel, chatID string) {
	if i := strings.Index(key, ":"); i >= 0 {
		return key[:i], key[i+1:]
	}
	return "cli", key
}
