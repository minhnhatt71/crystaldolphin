package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, dir string, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoad_NonExistent(t *testing.T) {
	cfg, err := Load("/nonexistent/path/config.json")
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	def := DefaultConfig()
	if cfg.Agents.Defaults.Model != def.Agents.Defaults.Model {
		t.Errorf("expected default model %q, got %q", def.Agents.Defaults.Model, cfg.Agents.Defaults.Model)
	}
}

func TestLoad_ValidConfig(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, map[string]any{
		"agents": map[string]any{
			"defaults": map[string]any{
				"model":     "openai/gpt-4o",
				"maxTokens": 4096,
			},
		},
	})

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Agents.Defaults.Model != "openai/gpt-4o" {
		t.Errorf("expected model %q, got %q", "openai/gpt-4o", cfg.Agents.Defaults.Model)
	}
	if cfg.Agents.Defaults.MaxTokens != 4096 {
		t.Errorf("expected maxTokens 4096, got %d", cfg.Agents.Defaults.MaxTokens)
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("{not valid json"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected no error for invalid JSON (falls back to default), got: %v", err)
	}
	def := DefaultConfig()
	if cfg.Agents.Defaults.Model != def.Agents.Defaults.Model {
		t.Errorf("expected default model %q, got %q", def.Agents.Defaults.Model, cfg.Agents.Defaults.Model)
	}
}

func TestLoad_EmptyPath(t *testing.T) {
	// Empty path should resolve to ConfigPath(); just verify it doesn't panic.
	// We can't control ~/.nanobot/config.json in tests, so we only check no panic/error crash.
	_, err := Load("")
	_ = err // may or may not exist on the test machine
}

func TestSave_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	original := DefaultConfig()
	original.Agents.Defaults.Model = "anthropic/claude-3-5-sonnet"
	original.Agents.Defaults.MaxTokens = 1234

	if err := Save(&original, path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.Agents.Defaults.Model != original.Agents.Defaults.Model {
		t.Errorf("model mismatch: got %q, want %q", loaded.Agents.Defaults.Model, original.Agents.Defaults.Model)
	}
	if loaded.Agents.Defaults.MaxTokens != original.Agents.Defaults.MaxTokens {
		t.Errorf("maxTokens mismatch: got %d, want %d", loaded.Agents.Defaults.MaxTokens, original.Agents.Defaults.MaxTokens)
	}
}

func TestSave_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := DefaultConfig()
	if err := Save(&cfg, path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("expected permissions 0600, got %04o", perm)
	}
}

func TestSave_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "dir", "config.json")

	cfg := DefaultConfig()
	if err := Save(&cfg, path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file not created: %v", err)
	}
}

func TestLoad_PartialConfig_UsesDefaults(t *testing.T) {
	dir := t.TempDir()
	// Only set one field; the rest should come from DefaultConfig.
	path := writeConfig(t, dir, map[string]any{
		"agents": map[string]any{
			"defaults": map[string]any{
				"model": "custom/model",
			},
		},
	})

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	def := DefaultConfig()
	if cfg.Agents.Defaults.Model != "custom/model" {
		t.Errorf("expected model %q, got %q", "custom/model", cfg.Agents.Defaults.Model)
	}
	// Unset fields should retain their defaults.
	if cfg.Agents.Defaults.Temperature != def.Agents.Defaults.Temperature {
		t.Errorf("expected default temperature %v, got %v", def.Agents.Defaults.Temperature, cfg.Agents.Defaults.Temperature)
	}
	if cfg.Agents.Defaults.MemoryWindow != def.Agents.Defaults.MemoryWindow {
		t.Errorf("expected default memoryWindow %d, got %d", def.Agents.Defaults.MemoryWindow, cfg.Agents.Defaults.MemoryWindow)
	}
}
