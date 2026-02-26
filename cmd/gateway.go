package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/crystaldolphin/crystaldolphin/internal/bus"
	"github.com/crystaldolphin/crystaldolphin/internal/channels"
	"github.com/crystaldolphin/crystaldolphin/internal/config"
	"github.com/crystaldolphin/crystaldolphin/internal/cron"
	"github.com/crystaldolphin/crystaldolphin/internal/dependency"
	"github.com/crystaldolphin/crystaldolphin/internal/heartbeat"
)

var (
	gatewayPort    int
	gatewayVerbose bool
)

var gatewayCmd = &cobra.Command{
	Use:   "gateway",
	Short: "Manage the crystaldolphin gateway server",
}

func init() {
	gatewayCmd.AddCommand(gatewayStartCmd)
	gatewayCmd.AddCommand(gatewayStopCmd)
	gatewayCmd.AddCommand(gatewayStatusCmd)

	gatewayStartCmd.Flags().IntVarP(&gatewayPort, "port", "p", 18790, "Gateway port")
	gatewayStartCmd.Flags().BoolVarP(&gatewayVerbose, "verbose", "v", false, "Verbose logging")
}

var gatewayStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the gateway server",
	RunE:  runGatewayStart,
}

func runGatewayStart(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load(config.ConfigPath())
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	svc, err := dependency.New(cfg)
	if err != nil {
		return err
	}

	fmt.Printf("%s Starting crystaldolphin gateway on port %d...\n", logo, gatewayPort)

	if err := writePIDFile(); err != nil {
		return err
	}
	defer removePIDFile()

	messageBus := svc.MessageBus()
	cronService := svc.CronService()
	loop := svc.AgentLoop()

	// Wire cron → agent callback.
	cronService.SetOnJob(func(ctx context.Context, job cron.CronJob) (string, error) {
		sessionKey := "cron:" + job.ID
		ch := ""
		chatID := "direct"
		if job.Payload.Channel != nil {
			ch = *job.Payload.Channel
		}
		if job.Payload.To != nil {
			chatID = *job.Payload.To
		}
		if ch == "" {
			ch = "cli"
		}

		resp := loop.ProcessDirect(ctx, job.Payload.Message, sessionKey, ch, chatID)
		if job.Payload.Deliver && job.Payload.To != nil {
			messageBus.PublishOutbound(bus.NewOutboundMessage(ch, chatID, resp))
		}
		return resp, nil
	})

	// Wire heartbeat → agent callback.
	hb := heartbeat.NewService(cfg.WorkspacePath(), func(ctx context.Context, content string) error {
		loop.ProcessDirect(ctx, content, "heartbeat:direct", "heartbeat", "direct")
		return nil
	}, 0)

	// Graceful shutdown context.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	g, gctx := errgroup.WithContext(ctx)

	channelMgr := channels.NewManager(cfg, messageBus)
	if enabled := channelMgr.EnabledChannels(); len(enabled) > 0 {
		fmt.Printf("✓ Channels enabled: %s\n", strings.Join(enabled, ", "))
	} else {
		fmt.Println("Warning: no channels enabled")
	}

	g.Go(func() error { return loop.Run(gctx) })
	g.Go(func() error { return cronService.Start(gctx) })
	g.Go(func() error { return hb.Start(gctx) })
	g.Go(func() error { return channelMgr.StartAll(gctx) })

	fmt.Printf("%s Gateway running. Press Ctrl+C to stop.\n", logo)

	if err := g.Wait(); err != nil && err != context.Canceled {
		fmt.Fprintf(os.Stderr, "gateway error: %v\n", err)
		return err
	}
	fmt.Println("\nShutdown complete.")
	return nil
}

var gatewayStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop a running gateway server",
	RunE: func(_ *cobra.Command, _ []string) error {
		pid, err := readPIDFile()
		if err != nil {
			return fmt.Errorf("gateway does not appear to be running: %w", err)
		}
		proc, err := os.FindProcess(pid)
		if err != nil {
			return fmt.Errorf("could not find process %d: %w", pid, err)
		}
		if err := proc.Signal(syscall.SIGTERM); err != nil {
			return fmt.Errorf("failed to stop gateway (pid %d): %w", pid, err)
		}
		fmt.Printf("✓ Sent SIGTERM to gateway (pid %d)\n", pid)
		return nil
	},
}

var gatewayStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show gateway status",
	RunE: func(_ *cobra.Command, _ []string) error {
		pid, err := readPIDFile()
		if err != nil {
			fmt.Println("Gateway: stopped")
			return nil
		}
		proc, err := os.FindProcess(pid)
		if err != nil {
			fmt.Println("Gateway: stopped")
			return nil
		}
		// On Linux, FindProcess always succeeds; send signal 0 to check liveness.
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			fmt.Println("Gateway: stopped")
			removePIDFile()
			return nil
		}
		fmt.Printf("Gateway: running (pid %d, port %d)\n", pid, gatewayPort)
		return nil
	},
}

func pidFilePath() string {
	return filepath.Join(config.DataDir(), "gateway.pid")
}

func writePIDFile() error {
	path := pidFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o644)
}

func removePIDFile() {
	_ = os.Remove(pidFilePath())
}

func readPIDFile() (int, error) {
	data, err := os.ReadFile(pidFilePath())
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}
