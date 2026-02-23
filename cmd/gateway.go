package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/crystaldolphin/crystaldolphin/internal/agent"
	"github.com/crystaldolphin/crystaldolphin/internal/bus"
	"github.com/crystaldolphin/crystaldolphin/internal/channels"
	"github.com/crystaldolphin/crystaldolphin/internal/config"
	"github.com/crystaldolphin/crystaldolphin/internal/cron"
	"github.com/crystaldolphin/crystaldolphin/internal/heartbeat"
)

var (
	gatewayPort    int
	gatewayVerbose bool
)

var gatewayCmd = &cobra.Command{
	Use:   "gateway",
	Short: "Start the crystaldolphin gateway server",
	RunE:  runGateway,
}

func init() {
	gatewayCmd.Flags().IntVarP(&gatewayPort, "port", "p", 18790, "Gateway port")
	gatewayCmd.Flags().BoolVarP(&gatewayVerbose, "verbose", "v", false, "Verbose logging")
}

func runGateway(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load(config.ConfigPath())
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	provider, err := buildProvider(cfg)
	if err != nil {
		return err
	}

	fmt.Printf("%s Starting crystaldolphin gateway on port %d...\n", logo, gatewayPort)

	b := bus.NewMessageBus(100)

	cronPath := config.DataDir() + "/cron/jobs.json"
	cronSvc := cron.NewService(cronPath)

	loop := agent.NewAgentLoop(b, provider, cfg, "")
	loop.SetCronTool(cronSvc)

	// Wire cron → agent callback.
	cronSvc.SetOnJob(func(ctx context.Context, job cron.CronJob) (string, error) {
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
			b.Outbound <- bus.OutboundMessage{
				Channel: ch,
				ChatID:  chatID,
				Content: resp,
			}
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

	channelMgr := channels.NewManager(cfg, b)
	if enabled := channelMgr.EnabledChannels(); len(enabled) > 0 {
		var sb string
		for i, n := range enabled {
			if i > 0 {
				sb += ", "
			}
			sb += n
		}
		fmt.Printf("✓ Channels enabled: %s\n", sb)
	} else {
		fmt.Println("Warning: no channels enabled")
	}

	g.Go(func() error { return loop.Run(gctx) })
	g.Go(func() error { return cronSvc.Start(gctx) })
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
