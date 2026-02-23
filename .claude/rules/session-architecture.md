# Session Architecture

## Overview

Sessions store per-conversation message history as JSONL files on disk. They are loaded on first access, kept in a memory cache, and written back after every agent turn.

## Components

| Component | File | Role |
|---|---|---|
| `Session` | `internal/session/manager.go` | In-memory conversation state |
| `Manager` | `internal/session/manager.go` | Load / save / cache sessions |

## Session key

The key is derived from the inbound message via `InboundMessage.SessionKey()`:

```
"channel:chat_id"   e.g.  "telegram:12345678"
                          "discord:987654321"
                          "cli:direct"
                          "cron:job-abc123"
```

The key is mapped to a JSONL filename by replacing `:` with `_` and sanitising unsafe characters:
```
"telegram:12345678"  →  telegram_12345678.jsonl
```

Files live in `<workspace>/sessions/`.

## JSONL file format

Line 1 is always a metadata object; all subsequent lines are message objects. This format is byte-compatible with the nanobot Python implementation.

```jsonl
{"_type":"metadata","key":"telegram:12345678","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-02-01T12:00:00Z","metadata":{},"last_consolidated":10}
{"role":"user","content":"Hello","timestamp":"2026-02-01T12:00:00Z"}
{"role":"assistant","content":"Hi there!","timestamp":"2026-02-01T12:00:01Z","tools_used":["web_search"]}
```

**`last_consolidated`** — index into the messages array up to which content has been summarised into `MEMORY.md`/`HISTORY.md`. Used by `memory.Consolidate()` to avoid re-summarising old turns.

## Lifecycle

```
Message arrives
    │
    ▼
Manager.GetOrCreate(key)
    ├── cache hit → return cached *Session
    └── cache miss → load from disk (or create empty)
              │
              ▼
Agent loop runs  (reads sess.GetHistory(memoryWindow))
              │
              ▼
sess.AddMessage("user", ...)
sess.AddMessage("assistant", ...)
              │
              ▼
Manager.Save(sess)   ← writes full JSONL to disk, updates cache
```

## In-memory cache

`Manager.cache` is a `sync.Map` (key → `*Session`). Multiple goroutines may call `GetOrCreate` concurrently; `LoadOrStore` ensures only one `*Session` per key is ever used, preventing duplicate state.

`Invalidate(key)` removes a session from the cache (called after `/new` so the next message starts clean).

## GetHistory — LLM-facing view

`Session.GetHistory(maxMessages int)` returns the last `maxMessages` entries with only LLM-relevant fields:

| Field kept | Purpose |
|---|---|
| `role` | `user` / `assistant` / `tool` |
| `content` | Message text |
| `tool_calls` | Assistant's tool invocation list |
| `tool_call_id` | Tool result routing |
| `name` | Tool name in result messages |

Fields stripped before sending to LLM: `timestamp`, `tools_used`, and any other session-only metadata.

## Memory consolidation

When `len(sess.Messages) > memoryWindow`, the agent loop triggers background consolidation (guarded by a per-session mutex flag in `AgentLoop.consolidating`):

```
go func() {
    mem.Consolidate(ctx, sess, provider, model, false, memoryWindow)
}()
```

`Consolidate()` (in `internal/agent/memory.go`) summarises older turns into `MEMORY.md` and `HISTORY.md` in the workspace, then advances `sess.LastConsolidated`.

The `/new` command also forces an immediate consolidation (with `force=true`) before clearing the session.

## Compatibility contracts

- Line 1 must be `{"_type":"metadata",...,"last_consolidated":N}`.
- Messages are **append-only** on disk (the file is always fully rewritten by `Save()`, but the logical content only grows).
- The `last_consolidated` counter must never exceed `len(messages)`.
- Key and filename derivation must match the Python nanobot logic exactly.
