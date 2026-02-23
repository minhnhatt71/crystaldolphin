# Agent Architecture

## Data flow

```
Chat platform
    │  (platform-specific listener)
    ▼
channels/*.go  ──HandleMessage()──▶  bus.Inbound  ──▶  agent/loop.go
                                                              │
                                                   runAgentLoop()
                                                              │
                                              providers/*.go (LLM call)
                                                              │
                                               tools/registry.go (tool calls)
                                                              │
                                                    bus.Outbound
                                                              │
                                          channels/manager.go ──Send()──▶  Chat platform
```

## Bus message types

**InboundMessage** (`internal/bus/events.go`) — a user message arriving from a channel:
- `Channel` — source channel name ("telegram", "discord", "cli", …)
- `SenderID` — user identifier within the channel
- `ChatID` — chat/DM/channel identifier
- `Content` — message text
- `Media` — local file paths of downloaded attachments
- `Metadata` — channel-specific extras (message_id, username, …)
- `SessionKey()` — returns `"channel:chat_id"`, used to load conversation history from `~/.nanobot/sessions/`

**OutboundMessage** (`internal/bus/events.go`) — the agent's reply going back to a channel:
- `Channel` — destination channel name
- `ChatID` — destination chat/DM identifier
- `Content` — text to send
- `ReplyTo` — original message ID to quote/thread (optional)
- `Media` — local file paths to attach (optional)
- `Metadata` — channel-specific hints (thread_ts, parse_mode, …)

## Direct processing (no bus)

`agent.ProcessDirect()` bypasses the bus entirely — used by `cmd/agent.go` (single-message CLI), cron jobs, and heartbeat. It runs the agent loop synchronously with a 5-minute timeout and returns the response string directly.
