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
	"exit": true, "quit": true, "/exit": true, "/quit": true, ":q": true,
}

func runAgent(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load(config.ConfigPath())
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if !agentLogs {
		// Suppress slog output by redirecting to discard.
		// Production code would set a no-op slog handler; keeping simple here.
	}

	container, err := dependency.New(cfg)
	if err != nil {
		return err
	}

	loop := container.AgentLoop()
	msgBus := container.MessageBus()

	sessionKey := agentSession
	channel, chatID := parseSessionKey(sessionKey)

	if agentMessage != "" {
		// Single message mode.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		fmt.Fprintf(os.Stderr, "  ↳ thinking...\n")
		resp := loop.ProcessDirect(ctx, agentMessage, sessionKey, channel, chatID)
		printResponse(resp)
		return nil
	}

	// Interactive mode.
	fmt.Printf("%s Interactive mode (type 'exit' or Ctrl+C to quit)\n\n", logo)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle SIGINT gracefully.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nGoodbye!")
		cancel()
		os.Exit(0)
	}()

	// Start agent loop in background.
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

		// Send to bus; wait for response.
		doneCh := make(chan struct{})
		msgBus.Inbound <- bus.InboundMessage{
			Channel:   channel,
			SenderID:  "user",
			ChatID:    chatID,
			Content:   line,
			Timestamp: time.Now(),
		}

		go func() {
			defer close(doneCh)
			for {
				select {
				case msg := <-msgBus.Outbound:
					if msg.Metadata != nil {
						if prog, _ := msg.Metadata["_progress"].(bool); prog {
							fmt.Printf("  ↳ %s\n", msg.Content)
							continue
						}
					}
					if msg.Content != "" {
						printResponse(msg.Content)
					}
					return
				case <-ctx.Done():
					return
				}
			}
		}()

		<-doneCh
	}
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
