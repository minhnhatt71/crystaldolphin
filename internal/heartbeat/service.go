// Package heartbeat provides a periodic background check that runs the agent
// against HEARTBEAT.md every 30 minutes when the file contains active tasks.
package heartbeat

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// OnHeartbeatFunc is called with the HEARTBEAT.md content when active tasks are found.
type OnHeartbeatFunc func(ctx context.Context, content string) error

// Service runs a periodic check of HEARTBEAT.md.
// Mirrors nanobot's Python HeartbeatService.
type Service struct {
	workspace   string
	onHeartbeat OnHeartbeatFunc
	interval    time.Duration
}

// NewService creates a HeartbeatService.
// interval defaults to 30 minutes if zero.
func NewService(workspace string, onHeartbeat OnHeartbeatFunc, interval time.Duration) *Service {
	if interval <= 0 {
		interval = 30 * time.Minute
	}

	return &Service{
		workspace:   workspace,
		onHeartbeat: onHeartbeat,
		interval:    interval,
	}
}

// Start runs the heartbeat loop until ctx is cancelled.
func (s *Service) Start(ctx context.Context) error {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	slog.Info("heartbeat: started", "interval", s.interval)

	for {
		select {
		case <-ticker.C:
			s.check(ctx)
		case <-ctx.Done():
			slog.Info("heartbeat: stopped")
			return ctx.Err()
		}
	}
}

func (s *Service) check(ctx context.Context) {
	path := filepath.Join(s.workspace, "HEARTBEAT.md")
	data, err := os.ReadFile(path)
	if err != nil {
		// File not found is normal — no heartbeat configured.
		return
	}

	content := string(data)
	if !hasActiveTasks(content) {
		return
	}

	slog.Info("heartbeat: active tasks found, running agent")
	if s.onHeartbeat != nil {
		if err := s.onHeartbeat(ctx, content); err != nil {
			slog.Error("heartbeat: agent error", "err", err)
		}
	}
}

// hasActiveTasks returns true when HEARTBEAT.md has at least one non-comment,
// non-empty, non-unchecked-checkbox line that is not a pure markdown heading.
//
// Mirrors Python's HeartbeatService.is_heartbeat_empty() logic (inverted):
// the file is "active" when it is NOT empty of tasks.
func hasActiveTasks(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "<!--") {
			continue
		}
		if strings.HasPrefix(trimmed, "- [ ]") {
			continue
		}
		if trimmed == "# HEARTBEAT" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		// Found a real line (checked box, text, etc.)
		return true
	}
	return false
}
