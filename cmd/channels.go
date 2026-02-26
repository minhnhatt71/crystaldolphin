package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/crystaldolphin/crystaldolphin/internal/config"
)

var channelsCmd = &cobra.Command{
	Use:   "channels",
	Short: "Manage chat channels",
}

func init() {
	channelsCmd.AddCommand(channelsStatusCmd)
	channelsCmd.AddCommand(channelsLoginCmd)
}

var channelsStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show channel status",
	RunE: func(_ *cobra.Command, _ []string) error {
		cfg, err := config.Load(config.ConfigPath())
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		type row struct{ name, enabled, detail string }
		rows := []row{
			{
				"WhatsApp",
				yesNo(cfg.Channels.WhatsApp.Enabled),
				cfg.Channels.WhatsApp.BridgeURL,
			},
			{
				"Telegram",
				yesNo(cfg.Channels.Telegram.Enabled),
				tokenHint(cfg.Channels.Telegram.Token),
			},
			{
				"Discord",
				yesNo(cfg.Channels.Discord.Enabled),
				cfg.Channels.Discord.GatewayURL,
			},
			{
				"Feishu",
				yesNo(cfg.Channels.Feishu.Enabled),
				tokenHint(cfg.Channels.Feishu.AppID),
			},
			{
				"DingTalk",
				yesNo(cfg.Channels.DingTalk.Enabled),
				tokenHint(cfg.Channels.DingTalk.ClientID),
			},
			{
				"Slack",
				yesNo(cfg.Channels.Slack.Enabled),
				func() string {
					if cfg.Channels.Slack.AppToken != "" && cfg.Channels.Slack.BotToken != "" {
						return "socket"
					}
					return "(not configured)"
				}(),
			},
			{
				"Email",
				yesNo(cfg.Channels.Email.Enabled),
				func() string {
					if cfg.Channels.Email.IMAPHost != "" {
						return cfg.Channels.Email.IMAPHost
					}
					return "(not configured)"
				}(),
			},
			{
				"Mochat",
				yesNo(cfg.Channels.Mochat.Enabled),
				cfg.Channels.Mochat.BaseURL,
			},
			{
				"QQ",
				yesNo(cfg.Channels.QQ.Enabled),
				tokenHint(cfg.Channels.QQ.AppID),
			},
		}

		fmt.Printf("%-12s %-8s %s\n", "Channel", "Enabled", "Configuration")
		fmt.Println(repeatStr("-", 60))
		for _, r := range rows {
			fmt.Printf("%-12s %-8s %s\n", r.name, r.enabled, r.detail)
		}
		return nil
	},
}

var channelsLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Link WhatsApp via QR code scan",
	RunE: func(_ *cobra.Command, _ []string) error {
		bridgeDir, err := getBridgeDir()
		if err != nil {
			return err
		}
		fmt.Printf("%s Starting bridge...\nScan the QR code to connect.\n\n", logo)
		cmd := exec.Command("npm", "start")
		cmd.Dir = bridgeDir
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	},
}

func getBridgeDir() (string, error) {
	home, _ := os.UserHomeDir()
	userBridge := filepath.Join(home, ".nanobot", "bridge")

	if _, err := os.Stat(filepath.Join(userBridge, "dist", "index.js")); err == nil {
		return userBridge, nil
	}

	if _, err := exec.LookPath("npm"); err != nil {
		return "", fmt.Errorf("npm not found — install Node.js >= 18")
	}

	// Try to find source bridge next to the binary.
	exe, _ := os.Executable()
	srcBridge := filepath.Join(filepath.Dir(exe), "bridge")
	if _, err := os.Stat(filepath.Join(srcBridge, "package.json")); err != nil {
		return "", fmt.Errorf("bridge source not found")
	}

	fmt.Printf("%s Setting up bridge...\n", logo)
	if err := os.MkdirAll(filepath.Dir(userBridge), 0o755); err != nil {
		return "", err
	}

	install := exec.Command("npm", "install")
	install.Dir = userBridge
	if out, err := install.CombinedOutput(); err != nil {
		return "", fmt.Errorf("npm install failed: %s", out)
	}
	build := exec.Command("npm", "run", "build")
	build.Dir = userBridge
	if out, err := build.CombinedOutput(); err != nil {
		return "", fmt.Errorf("npm build failed: %s", out)
	}
	fmt.Println("✓ Bridge ready")
	return userBridge, nil
}

func yesNo(b bool) string {
	if b {
		return "✓"
	}
	return "✗"
}

func tokenHint(s string) string {
	if s == "" {
		return "(not configured)"
	}

	if len(s) > 10 {
		return s[:10] + "..."
	}

	return s
}

// providerCmd handles OAuth logins.
var providerCmd = &cobra.Command{
	Use:   "provider",
	Short: "Manage LLM providers",
}

func init() {
	providerCmd.AddCommand(providerLoginCmd)
}

var providerLoginCmd = &cobra.Command{
	Use:   "login <provider>",
	Short: "Authenticate with an OAuth provider (e.g. openai-codex)",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		name := args[0]
		switch name {
		case "openai-codex", "openai_codex":
			return loginOpenAICodex()
		default:
			return fmt.Errorf("login not supported for provider %q", name)
		}
	},
}

func loginOpenAICodex() error {
	fmt.Println("OpenAI Codex OAuth login is not yet implemented in the Go version.")
	fmt.Println("Use the Python nanobot to obtain a token, then copy ~/.nanobot/codex_token.json")
	return nil
}
