# Gateway Architecture

## Overview

The gateway is the long-running server mode of crystaldolphin. It starts four concurrent goroutines — agent loop, cron service, heartbeat service, and channel manager — all sharing a single context and shutting down cleanly on SIGINT/SIGTERM.

## Components

| Component | File | Role |
|---|---|---|
| `runGatewayStart` | `cmd/gateway.go` | Entry point; wires everything and blocks via `errgroup` |
| `MessageBus` | `internal/bus/bus.go` | Two buffered channels (cap 100) connecting channels ↔ agent |
| `AgentLoop` | `internal/agent/loop.go` | Consumes `bus.Inbound`; runs LLM ↔ tool loop; publishes `bus.Outbound` |
| `Manager` (channels) | `internal/channels/manager.go` | Starts all enabled channels; routes `bus.Outbound` to the right channel |
| `JobManager` (cron) | `internal/cron/service.go` | Schedules periodic/one-shot jobs; calls back into `ProcessDirect` |
| `Service` (heartbeat) | `internal/heartbeat/service.go` | 30-min ticker; reads `HEARTBEAT.md`; calls back into `ProcessDirect` |

## Startup sequence (`gateway start`)

```
config.Load()
    │
buildProvider()          ← constructs LLMProvider from config
    │
writePIDFile()           ← writes ~/.nanobot/gateway.pid
    │
bus.NewMessageBus(100)   ← inbound cap=100, outbound cap=100
    │
cron.NewManager()        ← loads jobs.json; does NOT start timer yet
    │
agent.NewAgentLoop()     ← registers all tools; creates session manager
    │
loop.SetCronTool()       ← wires cron tool into agent registry
    │
cronSvc.SetOnJob()       ← wires job callback → loop.ProcessDirect()
    │
heartbeat.NewService()   ← wires heartbeat callback → loop.ProcessDirect()
    │
signal.NotifyContext()   ← intercepts SIGINT / SIGTERM
    │
channels.NewManager()    ← instantiates all enabled channel objects
    │
errgroup.Go × 4:
  ├── loop.Run(gctx)           agent loop
  ├── cronSvc.Start(gctx)      cron scheduler
  ├── hb.Start(gctx)           heartbeat ticker
  └── channelMgr.StartAll(gctx)  channel listeners + outbound dispatcher
    │
g.Wait()                 ← blocks until ctx cancelled or goroutine error
    │
removePIDFile()          ← cleanup on exit
```

## Full message flow

```
User sends a message on Telegram / Discord / etc.
    │
Channel.Start() listener
    │
Base.HandleMessage()           ← allowlist check; pushes to bus.Inbound
    │
bus.Inbound (buffered, cap 100)
    │
AgentLoop.Run() → processMessage()
    │
runAgentLoop()                 ← up to 20 LLM ↔ tool iterations
    │
bus.Outbound (buffered, cap 100)
    │
Manager.dispatchOutbound()     ← routes by msg.Channel field
    │
Channel.Send()                 ← platform-specific delivery
    │
User receives reply
```

## Cron-triggered flow

```
JobManager timer fires
    │
OnJobFunc(ctx, job)
    │
loop.ProcessDirect(ctx, job.Payload.Message, "cron:<jobID>", ch, chatID)
    │
(if job.Payload.Deliver == true)
    │
bus.Outbound ← OutboundMessage{Channel: ch, ChatID: chatID, Content: resp}
    │
Channel.Send()  →  user
```

## Heartbeat-triggered flow

```
time.Ticker fires (every 30 min)
    │
heartbeat.Service.check()
    ├── reads <workspace>/HEARTBEAT.md
    ├── hasActiveTasks() == false → skip (no-op)
    └── hasActiveTasks() == true
              │
              loop.ProcessDirect(ctx, content, "heartbeat:direct", "heartbeat", "direct")
              │
              agent loop runs; response is discarded (deliver=false)
```

`hasActiveTasks()` considers the file **inactive** if it contains only blank lines, HTML comments, unchecked checkboxes (`- [ ] …`), or Markdown headings. Any other line (plain text, checked box `- [x]`, etc.) makes it active.

## MessageBus

```go
type MessageBus struct {
    Inbound  chan InboundMessage   // channels → agent (cap 100)
    Outbound chan OutboundMessage  // agent → channels (cap 100)
}
```

`InboundMessage.SessionKey()` returns `"channel:chat_id"` — used by the agent to load the right JSONL session file.

## Direct processing (`ProcessDirect`)

Used by cron, heartbeat, and `cmd/agent.go` (CLI mode). Bypasses the bus entirely — runs the agent loop synchronously with a 5-minute timeout and returns the response string.

```
ProcessDirect(ctx, message, sessionKey, channel, chatID)
    │
    ├── loads/creates session for sessionKey
    ├── runs runAgentLoop() (max 20 iter)
    └── returns response string
```

## Concurrency model

| Goroutine | Blocking on |
|---|---|
| `loop.Run` | `bus.Inbound` (select loop); spawns per-message goroutines |
| `cronSvc.Start` | `ctx.Done()` + internal timers/robfig |
| `hb.Start` | `time.Ticker` + `ctx.Done()` |
| `channelMgr.StartAll` | `ctx.Done()`; starts per-channel goroutines + `dispatchOutbound` goroutine |

All four are managed by a single `errgroup`. Any goroutine returning a non-nil, non-`context.Canceled` error surfaces to `g.Wait()` and exits the process.

## PID file

| Action | File | Behaviour |
|---|---|---|
| `gateway start` | `~/.nanobot/gateway.pid` | Writes own PID; removed on clean exit |
| `gateway stop` | `~/.nanobot/gateway.pid` | Reads PID, sends `SIGTERM` |
| `gateway status` | `~/.nanobot/gateway.pid` | Reads PID, sends signal 0 to test liveness |

## Shutdown

1. SIGINT or SIGTERM → `signal.NotifyContext` cancels `ctx`.
2. `gctx` (derived from `ctx`) propagates cancellation to all four goroutines.
3. Each goroutine returns on `ctx.Done()`.
4. `g.Wait()` unblocks.
5. `removePIDFile()` runs via `defer`.

## Channel support matrix

| Channel | Transport | Split limit |
|---|---|---|
| Telegram | HTTP polling | 4000 chars |
| Discord | Gateway WebSocket | 2000 chars |
| Slack | Socket Mode | — |
| WhatsApp | WebSocket → local Node bridge (port 3001) | — |
| Feishu | WebSocket long connection | — |
| DingTalk | Stream Mode | — |
| Email | IMAP poll + SMTP | — |
| Mochat | HTTP polling | — |
| QQ | Gateway WebSocket | — |
