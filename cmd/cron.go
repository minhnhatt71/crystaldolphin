package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/crystaldolphin/crystaldolphin/internal/agent"
	"github.com/crystaldolphin/crystaldolphin/internal/bus"
	"github.com/crystaldolphin/crystaldolphin/internal/config"
	"github.com/crystaldolphin/crystaldolphin/internal/cron"
)

var cronCmd = &cobra.Command{
	Use:   "cron",
	Short: "Manage scheduled tasks",
}

func init() {
	cronCmd.AddCommand(cronListCmd)
	cronCmd.AddCommand(cronAddCmd)
	cronCmd.AddCommand(cronRemoveCmd)
	cronCmd.AddCommand(cronEnableCmd)
	cronCmd.AddCommand(cronRunCmd)
}

// ---- list ------------------------------------------------------------------

var cronListAll bool

var cronListCmd = &cobra.Command{
	Use:   "list",
	Short: "List scheduled jobs",
	RunE: func(_ *cobra.Command, _ []string) error {
		svc := cron.NewService(cronStorePath())
		jobs := svc.ListAllJobs(cronListAll)
		if len(jobs) == 0 {
			fmt.Println("No scheduled jobs.")
			return nil
		}
		fmt.Printf("%-10s %-20s %-25s %-10s %-20s\n", "ID", "Name", "Schedule", "Status", "Next Run")
		fmt.Println(repeatStr("-", 88))
		for _, j := range jobs {
			sched := formatSchedule(j.Schedule)
			status := "enabled"
			if !j.Enabled {
				status = "disabled"
			}
			nextRun := ""
			if j.State.NextRunAtMs != nil {
				t := time.UnixMilli(*j.State.NextRunAtMs)
				nextRun = t.Format("2006-01-02 15:04")
			}
			fmt.Printf("%-10s %-20s %-25s %-10s %-20s\n", j.ID, truncStr(j.Name, 19), truncStr(sched, 24), status, nextRun)
		}
		return nil
	},
}

func init() {
	cronListCmd.Flags().BoolVarP(&cronListAll, "all", "a", false, "Include disabled jobs")
}

var (
	cronAddName    string
	cronAddMsg     string
	cronAddEvery   int
	cronAddCron    string
	cronAddTZ      string
	cronAddAt      string
	cronAddDeliver bool
	cronAddTo      string
	cronAddChannel string
)

var cronAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a scheduled job",
	RunE: func(_ *cobra.Command, _ []string) error {
		if cronAddTZ != "" && cronAddCron == "" {
			return fmt.Errorf("--tz can only be used with --cron")
		}

		var kind string
		var everyMs int64
		var atMs int64

		switch {
		case cronAddEvery > 0:
			kind = "every"
			everyMs = int64(cronAddEvery) * 1000
		case cronAddCron != "":
			kind = "cron"
		case cronAddAt != "":
			kind = "at"
			dt, err := time.ParseInLocation("2006-01-02T15:04:05", cronAddAt, time.Local)
			if err != nil {
				dt, err = time.Parse(time.RFC3339, cronAddAt)
				if err != nil {
					return fmt.Errorf("invalid --at value %q: %w", cronAddAt, err)
				}
			}
			atMs = dt.UnixMilli()
		default:
			return fmt.Errorf("must specify --every, --cron, or --at")
		}

		svc := cron.NewService(cronStorePath())
		job, err := svc.AddJobFull(
			cronAddName, cronAddMsg, kind, everyMs, cronAddCron, cronAddTZ, atMs,
			cronAddDeliver, cronAddChannel, cronAddTo, kind == "at",
		)
		if err != nil {
			return err
		}
		fmt.Printf("✓ Added job '%s' (%s)\n", job.Name, job.ID)
		return nil
	},
}

func init() {
	cronAddCmd.Flags().StringVarP(&cronAddName, "name", "n", "", "Job name (required)")
	cronAddCmd.Flags().StringVarP(&cronAddMsg, "message", "m", "", "Message for agent (required)")
	cronAddCmd.Flags().IntVarP(&cronAddEvery, "every", "e", 0, "Run every N seconds")
	cronAddCmd.Flags().StringVarP(&cronAddCron, "cron", "c", "", "Cron expression (e.g. '0 9 * * *')")
	cronAddCmd.Flags().StringVar(&cronAddTZ, "tz", "", "IANA timezone for --cron")
	cronAddCmd.Flags().StringVar(&cronAddAt, "at", "", "Run once at ISO datetime")
	cronAddCmd.Flags().BoolVarP(&cronAddDeliver, "deliver", "d", false, "Deliver response to channel")
	cronAddCmd.Flags().StringVar(&cronAddTo, "to", "", "Recipient ID for delivery")
	cronAddCmd.Flags().StringVar(&cronAddChannel, "channel", "", "Channel for delivery")

	_ = cronAddCmd.MarkFlagRequired("name")
	_ = cronAddCmd.MarkFlagRequired("message")
}

var cronRemoveCmd = &cobra.Command{
	Use:   "remove <job-id>",
	Short: "Remove a scheduled job",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		svc := cron.NewService(cronStorePath())
		if svc.RemoveJob(args[0]) {
			fmt.Printf("✓ Removed job %s\n", args[0])
		} else {
			fmt.Printf("Job %s not found\n", args[0])
		}
		return nil
	},
}

var cronEnableDisable bool

var cronEnableCmd = &cobra.Command{
	Use:   "enable <job-id>",
	Short: "Enable (or disable) a job",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		svc := cron.NewService(cronStorePath())
		job, ok := svc.EnableJob(args[0], !cronEnableDisable)
		if !ok {
			fmt.Printf("Job %s not found\n", args[0])
			return nil
		}
		action := "enabled"
		if cronEnableDisable {
			action = "disabled"
		}
		fmt.Printf("✓ Job '%s' %s\n", job.Name, action)
		return nil
	},
}

func init() {
	cronEnableCmd.Flags().BoolVar(&cronEnableDisable, "disable", false, "Disable instead of enable")
}

var cronRunForce bool

var cronRunCmd = &cobra.Command{
	Use:   "run <job-id>",
	Short: "Manually run a job",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		cfg, err := config.Load(config.ConfigPath())
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		provider, err := buildProvider(cfg)
		if err != nil {
			return err
		}

		b := bus.NewMessageBus(100)
		loop := agent.NewAgentLoop(b, provider, cfg, "")

		svc := cron.NewService(cronStorePath())
		svc.SetOnJob(func(ctx context.Context, job cron.CronJob) (string, error) {
			ch := "cli"
			chatID := "direct"
			if job.Payload.Channel != nil {
				ch = *job.Payload.Channel
			}
			if job.Payload.To != nil {
				chatID = *job.Payload.To
			}
			resp := loop.ProcessDirect(ctx, job.Payload.Message, "cron:"+job.ID, ch, chatID)
			if resp != "" {
				printResponse(resp)
			}
			return resp, nil
		})

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		if svc.RunJob(ctx, args[0], cronRunForce) {
			fmt.Println("✓ Job executed")
		} else {
			fmt.Printf("Failed to run job %s (not found or disabled; use --force)\n", args[0])
		}
		return nil
	},
}

func init() {
	cronRunCmd.Flags().BoolVarP(&cronRunForce, "force", "f", false, "Run even if disabled")
}

// ---- helpers ---------------------------------------------------------------

func cronStorePath() string { return config.DataDir() + "/cron/jobs.json" }

func formatSchedule(s cron.CronSchedule) string {
	switch s.Kind {
	case "every":
		if s.EveryMs != nil {
			return fmt.Sprintf("every %ds", *s.EveryMs/1000)
		}
	case "cron":
		if s.Expr != nil {
			if s.TZ != nil {
				return *s.Expr + " (" + *s.TZ + ")"
			}
			return *s.Expr
		}
	case "at":
		return "one-time"
	}
	return s.Kind
}

func truncStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

func repeatStr(s string, n int) string {
	var b string
	for i := 0; i < n; i++ {
		b += s
	}
	return b
}
