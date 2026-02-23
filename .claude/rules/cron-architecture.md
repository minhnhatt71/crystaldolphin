# Cron Architecture

## Overview

The cron system lets the agent (via the `cron` tool) schedule recurring tasks, one-time reminders, and interval jobs. When a job fires, it calls back into the agent loop and optionally delivers the response to the original chat.

## Components

| Component | File | Role |
|---|---|---|
| `CronTool` | `internal/tools/cron.go` | LLM-callable tool; parses `add/list/remove` actions |
| `CronServicer` | `internal/tools/cron.go` | Interface between tool and service (avoids import cycle) |
| `JobManager` | `internal/cron/service.go` | Scheduler; owns persistence, timers, robfig entries |
| `CronSchedule` | `internal/cron/service.go` | Schedule definition stored in `jobs.json` |

## Data flow

```
LLM calls cron tool
    │
    ▼
CronTool.Execute()   (action: add / list / remove)
    │
    ▼
CronServicer.AddJob() / ListJobs() / RemoveJob()
    │  (implemented by JobManager)
    ▼
JobManager   ──saves──▶  ~/.nanobot/cron/jobs.json
    │
    │  (when job fires)
    ▼
OnJobFunc callback   ──▶  agent.ProcessDirect()   ──▶  LLM response
    │
    └──(if deliver=true)──▶  bus.Outbound   ──▶  chat platform
```

## Schedule kinds

| Kind | Field | Description |
|---|---|---|
| `every` | `everyMs` | Interval in milliseconds; re-arms after each execution |
| `cron` | `expr` + `tz` | Standard 5-field cron expression (min/hr/dom/mon/dow); IANA timezone |
| `at` | `atMs` | One-time Unix timestamp (ms); `deleteAfterRun=true` by default |

## Timezone support

- Only `cron`-kind jobs support timezone via the `tz` field (IANA name, e.g. `"Asia/Ho_Chi_Minh"`).
- Used in two places:
  1. `armJobLocked` — wraps the robfig schedule with `withLocation()` so it fires at the correct wall-clock time.
  2. `computeNextRun` — computes the `nextRunAtMs` preview stored in `jobs.json`.
- Falls back to `time.Local` if `tz` is absent, empty, or invalid.

## Persistence — `jobs.json`

```jsonc
{
  "version": 1,
  "jobs": [
    {
      "id": "a1b2c3d4",
      "name": "Daily standup",
      "enabled": true,
      "schedule": { "kind": "cron", "expr": "0 9 * * *", "tz": "Asia/Ho_Chi_Minh" },
      "payload": {
        "kind": "agent_turn",
        "message": "Send standup reminder",
        "deliver": true,
        "channel": "telegram",
        "to": "12345"
      },
      "state": { "nextRunAtMs": 1234567890000, "lastRunAtMs": null, "lastStatus": null },
      "createdAtMs": 1234567800000,
      "updatedAtMs": 1234567800000,
      "deleteAfterRun": false
    }
  ]
}
```

**Compatibility**: key names and structure are byte-compatible with nanobot Python `jobs.json`.

## Internal scheduling

- **`every`** jobs use `time.AfterFunc`; re-armed inside the callback after each execution.
- **`at`** jobs use `time.AfterFunc` with a one-shot delay; not re-armed.
- **`cron`** jobs use `github.com/robfig/cron/v3` with `WithSeconds()` disabled (5-field parser) and a `locSchedule` wrapper for timezone.

Robfig entries and `time.Timer`s are tracked separately:

```
JobManager.timers    map[jobID]*time.Timer        (every + at)
JobManager.robfigIDs map[jobID]robfigcron.EntryID (cron)
```

`cancelTimerLocked(id)` stops both and removes them from their maps.

## CronTool parameter mapping

| Tool parameter | Kind triggered | Mapped to |
|---|---|---|
| `every_seconds` | `every` | `everyMs = every_seconds * 1000` |
| `cron_expr` (+ optional `tz`) | `cron` | `expr`, `tz` |
| `at` (ISO datetime string) | `at` | `atMs`; `deleteAfterRun=true` |

The `at` field accepts RFC3339 (`2026-02-12T10:30:00Z`) or local datetime (`2026-02-12T10:30:00`).

## Lifecycle

```
NewManager(storePath)         ← called in gateway start
SetOnJob(fn)                  ← wires in agent.ProcessDirect callback
Start(ctx)                    ← loads jobs.json, recomputes nextRun, arms timers
                              ← blocks until ctx cancelled
<ctx.Done>                    ← robfig stopped, timers cancelled
```

All mutations (`AddJob`, `RemoveJob`, `EnableJob`) are protected by `JobManager.mu` and immediately call `saveLocked()`.
