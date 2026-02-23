# Heartbeat Architecture

## Overview

The heartbeat service is a background timer that checks `HEARTBEAT.md` every 30 minutes. If the file contains active tasks, it passes the file's content directly to the agent loop — enabling autonomous, periodic agent work without any user interaction.

## Components

| Component | File | Role |
|---|---|---|
| `Service` | `internal/heartbeat/service.go` | Ticker loop; reads and evaluates `HEARTBEAT.md` |
| `OnHeartbeatFunc` | `internal/heartbeat/service.go` | Callback injected at startup; calls `agent.ProcessDirect()` |
| `HEARTBEAT.md` | `<workspace>/HEARTBEAT.md` | User-editable task list read on every tick |

## Data flow

```
time.Ticker fires (every 30 min)
    │
    ▼
Service.check()
    ├── reads <workspace>/HEARTBEAT.md
    ├── file missing → skip (no-op)
    ├── hasActiveTasks() == false → skip
    └── hasActiveTasks() == true
              │
              ▼
        OnHeartbeatFunc(ctx, content)
              │
              ▼
        agent.ProcessDirect(ctx, content, "heartbeat:direct", "heartbeat", "direct")
              │
              ▼
        agent loop runs (same LLM + tool loop as a normal message)
              └── response is discarded (deliver=false by default)
```

## `hasActiveTasks` — what counts as active

`HEARTBEAT.md` is considered **empty** (no-op) when it contains only:

- Blank lines
- HTML comments (`<!-- … -->`)
- Unchecked checkboxes (`- [ ] …`)
- Markdown headings (`# …`)

Any other line (checked box `- [x]`, plain text, etc.) makes the file **active** and triggers the agent.

## `HEARTBEAT.md` format

```markdown
# Heartbeat Tasks

## Active Tasks

<!-- Add periodic tasks below -->
- [ ] Check for unread emails and summarise
- Send a daily weather summary to Telegram

## Completed

<!-- Move finished tasks here -->
```

Only the "Active Tasks" section matters functionally. The file is passed verbatim as the user message to the agent, so any instructions written there are interpreted by the LLM.

## Wiring (gateway start)

```go
hb := heartbeat.NewService(cfg.WorkspacePath(), func(ctx context.Context, content string) error {
    loop.ProcessDirect(ctx, content, "heartbeat:direct", "heartbeat", "direct")
    return nil
}, 0)  // interval=0 → defaults to 30 minutes

g.Go(func() error { return hb.Start(gctx) })
```

The heartbeat runs as one of four concurrent goroutines in the gateway's errgroup (alongside the agent loop, cron service, and channel manager). It shares the same context and shuts down cleanly on SIGINT/SIGTERM.

## Session key

Heartbeat turns use the fixed session key `"heartbeat:direct"`, so the agent remembers context across heartbeat invocations (memory consolidation applies normally).

## Limits and notes

- The heartbeat interval is always 30 minutes (not user-configurable at runtime; change `NewService` call to adjust).
- `HEARTBEAT.md` is re-read on every tick — edits take effect at the next interval with no restart required.
- If `ProcessDirect` returns before the next tick, the callback completes synchronously; long-running LLM calls do not block the ticker itself (the ticker fires regardless).
- No result is delivered back to any channel; the agent runs for its own effect (e.g. writing files, calling tools, sending messages via the `message` tool if explicitly instructed).
