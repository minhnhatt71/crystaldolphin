package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/crystaldolphin/crystaldolphin/internal/interfaces"
)

// CronJobSummary is a lightweight view of a cron job used by the tool.
// The canonical definition lives in internal/interfaces; this alias keeps
// existing code compiling without changes.
type CronJobSummary = interfaces.CronJobSummary

// Service is the interface the CronTool uses to interact with the cron service.
// The canonical definition lives in internal/interfaces; this alias keeps
// existing code compiling without changes.
type Service = interfaces.CronService

// CronTool allows the agent to schedule reminders and recurring tasks.
type CronTool struct {
	svc     Service
	channel string
	chatID  string
}

// NewCronTool creates a CronTool backed by the given CronTool.
func NewCronTool(svc Service) *CronTool {
	return &CronTool{svc: svc}
}

// SetContext updates the channel/chatID for delivery before each turn.
func (t *CronTool) SetContext(channel, chatID string) {
	t.channel = channel
	t.chatID = chatID
}

func (t *CronTool) Name() string { return "cron" }

func (t *CronTool) Description() string {
	return "Schedule reminders and recurring tasks. Actions: add, list, remove."
}

func (t *CronTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {
				"type": "string",
				"enum": ["add", "list", "remove"],
				"description": "Action to perform"
			},
			"message": {
				"type": "string",
				"description": "Reminder message (for add)"
			},
			"every_seconds": {
				"type": "integer",
				"description": "Interval in seconds (for recurring tasks)"
			},
			"cron_expr": {
				"type": "string",
				"description": "Cron expression like '0 9 * * *' (for scheduled tasks)"
			},
			"tz": {
				"type": "string",
				"description": "IANA timezone for cron expressions (e.g. 'America/Vancouver')"
			},
			"at": {
				"type": "string",
				"description": "ISO datetime for one-time execution (e.g. '2026-02-12T10:30:00')"
			},
			"job_id": {
				"type": "string",
				"description": "Job ID (for remove)"
			}
		},
		"required": ["action"]
	}`)
}

func (t *CronTool) Execute(_ context.Context, params map[string]any) (string, error) {
	action, _ := params["action"].(string)
	switch action {
	case "add":
		return t.addJob(params), nil
	case "list":
		return t.listJobs(), nil
	case "remove":
		return t.removeJob(params), nil
	default:
		return fmt.Sprintf("Unknown action: %s", action), nil
	}
}

func (t *CronTool) addJob(params map[string]any) string {
	message, _ := params["message"].(string)
	if message == "" {
		return "Error: message is required for add"
	}
	if t.channel == "" || t.chatID == "" {
		return "Error: no session context (channel/chat_id)"
	}

	var kind string
	var everyMs, atMs int64
	var cronExpr, tz string
	deleteAfterRun := false

	if v, ok := numericToInt64(params["every_seconds"]); ok && v > 0 {
		kind = "every"
		everyMs = v * 1000
	} else if expr, ok := params["cron_expr"].(string); ok && expr != "" {
		kind = "cron"
		cronExpr = expr
		if tzVal, ok := params["tz"].(string); ok {
			tz = tzVal
		}
	} else if atStr, ok := params["at"].(string); ok && atStr != "" {
		dt, err := time.Parse(time.RFC3339, atStr)
		if err != nil {
			// Try without timezone (local)
			dt, err = time.ParseInLocation("2006-01-02T15:04:05", atStr, time.Local)
			if err != nil {
				return fmt.Sprintf("Error: invalid 'at' datetime %q: %v", atStr, err)
			}
		}
		kind = "at"
		atMs = dt.UnixMilli()
		deleteAfterRun = true
	} else {
		return "Error: either every_seconds, cron_expr, or at is required"
	}

	name := message
	if len(name) > 30 {
		name = name[:30]
	}

	id, err := t.svc.AddJob(name, message, kind, everyMs, cronExpr, tz, atMs,
		true, t.channel, t.chatID, deleteAfterRun)
	if err != nil {
		return fmt.Sprintf("Error creating job: %v", err)
	}
	return fmt.Sprintf("Created job '%s' (id: %s)", name, id)
}

func (t *CronTool) listJobs() string {
	jobs := t.svc.ListJobs()
	if len(jobs) == 0 {
		return "No scheduled jobs."
	}
	var sb string
	sb = "Scheduled jobs:\n"
	for _, j := range jobs {
		sb += fmt.Sprintf("- %s (id: %s, %s)\n", j.Name, j.ID, j.Kind)
	}
	return sb
}

func (t *CronTool) removeJob(params map[string]any) string {
	jobID, _ := params["job_id"].(string)
	if jobID == "" {
		return "Error: job_id is required for remove"
	}
	if t.svc.RemoveJob(jobID) {
		return fmt.Sprintf("Removed job %s", jobID)
	}
	return fmt.Sprintf("Job %s not found", jobID)
}

// numericToInt64 converts float64 or int from JSON params to int64.
func numericToInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int:
		return int64(n), true
	case int64:
		return n, true
	}
	return 0, false
}
