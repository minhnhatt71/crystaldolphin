package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/crystaldolphin/crystaldolphin/internal/config"
	"github.com/crystaldolphin/crystaldolphin/internal/providers"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show crystaldolphin status",
	RunE:  runStatus,
}

func runStatus(_ *cobra.Command, _ []string) error {
	cfgPath := config.ConfigPath()

	fmt.Printf("%s crystaldolphin Status\n\n", logo)

	_, statErr := os.Stat(cfgPath)
	cfgMark := "✗"
	if statErr == nil {
		cfgMark = "✓"
	}
	fmt.Printf("Config:    %s %s\n", cfgPath, cfgMark)

	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Printf("  (could not load config: %v)\n", err)
		return nil
	}

	ws := cfg.WorkspacePath()
	_, wsErr := os.Stat(ws)
	wsMark := "✗"
	if wsErr == nil {
		wsMark = "✓"
	}

	fmt.Printf("Workspace: %s %s\n", ws, wsMark)
	fmt.Printf("Model:     %s\n\n", cfg.Agents.Defaults.Model)

	fmt.Println("Providers:")
	for _, spec := range providers.PROVIDERS {
		p := cfg.ProviderByName(spec.Name)
		if p == nil {
			continue
		}
		label := spec.Label()
		switch {
		case spec.IsOAuth:
			fmt.Printf("  %-20s ✓ (OAuth)\n", label)
		case spec.IsLocal:
			if p.APIBase != "" {
				fmt.Printf("  %-20s ✓ %s\n", label, p.APIBase)
			} else {
				fmt.Printf("  %-20s (not set)\n", label)
			}
		default:
			if p.APIKey != "" {
				fmt.Printf("  %-20s ✓\n", label)
			} else {
				fmt.Printf("  %-20s (not set)\n", label)
			}
		}
	}
	return nil
}
