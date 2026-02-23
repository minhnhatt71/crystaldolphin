package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/crystaldolphin/crystaldolphin/internal/config"
)

var onboardCmd = &cobra.Command{
	Use:   "onboard",
	Short: "Initialize configuration and workspace",
	RunE:  runOnboard,
}

func runOnboard(_ *cobra.Command, _ []string) error {
	cfgPath := config.ConfigPath()

	if _, err := os.Stat(cfgPath); err == nil {
		fmt.Printf("Config already exists at %s\n", cfgPath)
		fmt.Printf("Press Enter to refresh (keep existing values) or Ctrl+C to cancel: ")
		fmt.Scanln()
		existing, loadErr := config.Load(cfgPath)
		if loadErr != nil {
			def := config.DefaultConfig()
			existing = &def
		}
		if err := config.Save(existing, cfgPath); err != nil {
			return err
		}
		fmt.Printf("✓ Config refreshed at %s\n", cfgPath)
	} else {
		cfg := config.DefaultConfig()
		if err := config.Save(&cfg, cfgPath); err != nil {
			return err
		}
		fmt.Printf("✓ Created config at %s\n", cfgPath)
	}

	def := config.DefaultConfig()
	workspace := def.WorkspacePath()
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}
	fmt.Printf("✓ Workspace at %s\n", workspace)

	createWorkspaceTemplates(workspace)

	fmt.Printf("\n%s crystaldolphin is ready!\n\n", logo)
	fmt.Println("Next steps:")
	fmt.Printf("  1. Add your API key to %s\n", cfgPath)
	fmt.Println("     Get one at: https://openrouter.ai/keys")
	fmt.Printf("  2. Chat: crystaldolphin agent -m \"Hello!\"\n")
	return nil
}

func createWorkspaceTemplates(workspace string) {
	templates := map[string]string{
		"AGENTS.md": `# Agent Instructions

You are a helpful AI assistant. Be concise, accurate, and friendly.

## Guidelines

- Always explain what you're doing before taking actions
- Ask for clarification when the request is ambiguous
- Use tools to help accomplish tasks
- Remember important information in memory/MEMORY.md; past events are logged in memory/HISTORY.md
`,
		"SOUL.md": `# Soul

I am crystaldolphin, a lightweight AI assistant.

## Personality

- Helpful and friendly
- Concise and to the point
- Curious and eager to learn

## Values

- Accuracy over speed
- User privacy and safety
- Transparency in actions
`,
		"USER.md": `# User

Information about the user goes here.

## Preferences

- Communication style: (casual/formal)
- Timezone: (your timezone)
- Language: (your preferred language)
`,
	}

	for filename, content := range templates {
		p := filepath.Join(workspace, filename)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			_ = os.WriteFile(p, []byte(content), 0o644)
			fmt.Printf("  Created %s\n", filename)
		}
	}

	memDir := filepath.Join(workspace, "memory")
	_ = os.MkdirAll(memDir, 0o755)

	memFile := filepath.Join(memDir, "MEMORY.md")
	if _, err := os.Stat(memFile); os.IsNotExist(err) {
		_ = os.WriteFile(memFile, []byte(`# Long-term Memory

This file stores important information that should persist across sessions.

## User Information

(Important facts about the user)

## Preferences

(User preferences learned over time)

## Important Notes

(Things to remember)
`), 0o644)
		fmt.Println("  Created memory/MEMORY.md")
	}

	histFile := filepath.Join(memDir, "HISTORY.md")
	if _, err := os.Stat(histFile); os.IsNotExist(err) {
		_ = os.WriteFile(histFile, []byte(""), 0o644)
		fmt.Println("  Created memory/HISTORY.md")
	}

	_ = os.MkdirAll(filepath.Join(workspace, "skills"), 0o755)
}
