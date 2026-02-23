# Subagent Architecture

## Overview

Subagents are background goroutines spawned by the main agent to handle complex or long-running tasks asynchronously. The caller (LLM) returns immediately; the subagent runs its own isolated LLM ↔ tool loop and reports back when done.

## Components

| Component | File | Role |
|---|---|---|
| `SpawnTool` | `internal/tools/spawn.go` | LLM-callable tool; entry point for spawning |
| `Spawner` | `internal/tools/spawn.go` | Interface (avoids import cycle with agent package) |
| `SubagentManager` | `internal/agent/subagent.go` | Implements `Spawner`; owns goroutine lifecycle |

## Data flow

```
LLM calls spawn tool  {task: "...", label: "..."}
    │
    ▼
SpawnTool.Execute()
    │
    ▼
SubagentManager.Spawn()
    ├── generates taskID (8-char hex)
    ├── creates detached context (context.Background())  ← survives parent request
    ├── stores cancel func in running map
    ├── go runSubagent()                                 ← returns immediately
    └── returns "Subagent [label] started (id: xxxx)"
              │
              ▼  (background goroutine)
         executeTask()   ← isolated tool registry + own LLM loop (max 15 iter)
              │
              ▼
         announceResult()
              │
              ▼
         bus.Inbound ← InboundMessage{Channel: "system", SenderID: "subagent"}
              │
              ▼
         agent loop handles as system message → LLM summarises → user sees result
```

## Isolation

Each subagent creates its **own tool registry** — a strict subset of the main agent's tools:

| Tool | Subagent has it |
|---|---|
| `read_file` | Yes |
| `write_file` | Yes |
| `edit_file` | Yes |
| `list_dir` | Yes |
| `exec` | Yes |
| `web_search` | Yes |
| `web_fetch` | Yes |
| `message` | **No** — cannot send messages directly |
| `spawn` | **No** — cannot spawn further subagents |
| `cron` | **No** |

No session history is shared. The subagent starts fresh with only a system prompt and the task string as the user message.

## System prompt

Built by `buildPrompt()` (`internal/agent/subagent.go`). Includes:

- Current time and timezone
- Role definition ("you are a subagent")
- Rules: stay focused, no side tasks, no direct messaging
- Available capabilities
- Workspace path and OS info

## Result delivery

`announceResult()` injects a **system-channel InboundMessage** back into `bus.Inbound`:

```go
bus.Inbound <- InboundMessage{
    Channel:  "system",
    SenderID: "subagent",
    ChatID:   originChannel + ":" + originChatID,  // original user's location
    Content:  "[Subagent 'label' completed/failed]\nTask: ...\nResult: ...\n\nSummarise naturally...",
}
```

The main agent loop picks this up in `handleSystemMessage()` (`internal/agent/loop.go:318`), runs one more LLM call to produce a user-friendly summary, and sends the reply to the correct channel/chat.

## Context and cancellation

- Subagent context is **detached** from the caller: `context.WithCancel(context.Background())`.
- The cancel func is stored in `SubagentManager.running[taskID]`.
- On completion (success or error), the goroutine removes itself from `running` and calls `cancel()`.
- `RunningCount()` returns the number of currently active subagents.

## Limits

| Property | Value |
|---|---|
| Max tool iterations per subagent | 15 (vs 20 for main agent) |
| No recursion | Subagents cannot spawn subagents |
| No history | Fresh message list per task |
| Same LLM/model | Inherits provider, model, temperature, maxTokens from main agent |
