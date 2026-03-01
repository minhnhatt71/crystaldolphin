package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/crystaldolphin/crystaldolphin/internal/bus"
	"github.com/crystaldolphin/crystaldolphin/internal/channels"
	"github.com/crystaldolphin/crystaldolphin/internal/config"
	"github.com/crystaldolphin/crystaldolphin/internal/dependency"
	"github.com/crystaldolphin/crystaldolphin/internal/schema"
	"github.com/crystaldolphin/crystaldolphin/internal/shared/cmdutils"
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

func runAgent(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load(config.ConfigPath())
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	container, err := dependency.New(cfg)
	if err != nil {
		return err
	}

	channel, chatId := bus.ParseRoutingKey(key)

	loop := container.AgentLoop()

	if message != "" {
		return runSingleMessage(loop, key, channel, chatId)
	}

	return runInteractive(loop, container.AgentBus(), container.ConsoleBus())
}

// runSingleMessage sends one message to the agent and prints the response.
func runSingleMessage(loop schema.AgentLooper, key string, channel bus.Channel, chatId string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	msg := bus.NewAgentMessage(channel, "user", chatId, message, key)

	fmt.Fprintf(os.Stderr, "  ↳ thinking...\n")

	res := loop.ProcessDirect(ctx, msg)

	cmdutils.PrintResponse(res)

	return nil
}

// runInteractive starts the agent loop and delegates the REPL to CLIChannel.
func runInteractive(loop schema.AgentLooper, inbound *bus.AgentBus, console *bus.ConsoleBus) error {
	fmt.Printf("%s Interactive mode (type 'exit' or Ctrl+C to quit)\n\n", logo)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	registerUserSignals(cancel)

	go func() { _ = loop.Run(ctx) }()

	cli := channels.NewCLIChannel(inbound, console)

	return cli.Start(ctx)
}

// registerUserSignals cancels ctx on SIGINT or SIGTERM and exits.
func registerUserSignals(cancel context.CancelFunc) {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		switch <-signalChan {
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
