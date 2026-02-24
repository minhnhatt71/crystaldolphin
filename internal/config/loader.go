package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ConfigPath returns the default configuration file path: ~/.nanobot/config.json.
func ConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".nanobot/config.json"
	}
	return filepath.Join(home, ".nanobot", "config.json")
}

// DataDir returns the nanobot data directory: ~/.nanobot.
func DataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".nanobot"
	}
	return filepath.Join(home, ".nanobot")
}

// Load reads and parses the config file at path.
// If path is empty, ConfigPath() is used.
// On parse failure it prints a warning and returns DefaultConfig().
func Load(path string) (*Config, error) {
	if path == "" {
		path = ConfigPath()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := DefaultConfig()
			return &cfg, nil
		}
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, &cfg); err != nil {
		fmt.Printf("Warning: failed to parse config %s: %v\n", path, err)
		fmt.Println("Using default configuration.")
		cfg2 := DefaultConfig()
		return &cfg2, nil
	}

	return &cfg, nil
}

// Save writes cfg to path as indented JSON.
// If path is empty, ConfigPath() is used.
func Save(cfg *Config, path string) error {
	if path == "" {
		path = ConfigPath()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	// Append a trailing newline for POSIX compliance.
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}
	return nil
}
