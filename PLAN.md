# Crystaldolphin — Go Rewrite Plan

## What & Why

Rewrite **nanobot** (Python, ~4k lines) as **crystaldolphin** in Go.
Goals: single static binary, lower memory, goroutine concurrency, easy cross-platform distribution.
Constraint: preserve all behavior — session files, config format, tool schemas, channel UX.

Config file stays at `~/.nanobot/config.json` (same camelCase keys).

---

## Project Layout

```
crystaldolphin/
├── main.go
├── go.mod
├── Makefile
├── cmd/
│   ├── root.go        # cobra root
│   ├── onboard.go     # nanobot onboard
│   ├── agent.go       # nanobot agent [-m "..."]
│   ├── gateway.go     # nanobot gateway
│   ├── status.go      # nanobot status
│   ├── cron.go        # nanobot cron list|add|remove|enable|run
│   └── channels.go    # nanobot channels status|login
├── internal/
│   ├── bus/
│   │   ├── events.go       # InboundMessage, OutboundMessage
│   │   └── bus.go          # MessageBus (buffered channels)
│   ├── config/
│   │   ├── schema.go       # Config structs (camelCase json tags)
│   │   └── loader.go       # Load/save ~/.nanobot/config.json
│   ├── session/
│   │   └── manager.go      # JSONL session storage
│   ├── agent/
│   │   ├── loop.go         # Core agent loop
│   │   ├── context.go      # System prompt + message builder
│   │   ├── memory.go       # MEMORY.md + HISTORY.md + consolidation
│   │   ├── skills.go       # SKILL.md loader + XML summary
│   │   └── subagent.go     # Background goroutine tasks
│   ├── tools/
│   │   ├── registry.go     # Tool interface + registry
│   │   ├── shell.go        # ExecTool (deny patterns)
│   │   ├── filesystem.go   # ReadFile/WriteFile/EditFile/ListDir
│   │   ├── web.go          # WebSearch (Brave) + WebFetch (readability)
│   │   ├── message.go      # MessageTool (outbound routing)
│   │   ├── spawn.go        # SpawnTool (subagent launch)
│   │   ├── cron.go         # CronTool (add/list/remove)
│   │   └── mcp.go          # MCP stdio + HTTP client
│   ├── providers/
│   │   ├── registry.go     # 18 ProviderSpec entries
│   │   ├── provider.go     # LLMProvider interface + LLMResponse
│   │   ├── openai.go       # OpenAI-compatible HTTP (covers all providers)
│   │   └── codex.go        # OpenAI Codex OAuth + SSE
│   ├── channels/
│   │   ├── base.go         # Channel interface + allowlist
│   │   ├── manager.go      # Start all + dispatch outbound
│   │   ├── telegram.go
│   │   ├── discord.go      # Raw Gateway WebSocket
│   │   ├── slack.go        # Socket Mode
│   │   ├── whatsapp.go     # WS client → Node.js bridge
│   │   ├── feishu.go
│   │   ├── dingtalk.go
│   │   ├── email.go        # IMAP poll + SMTP
│   │   ├── mochat.go
│   │   └── qq.go
│   ├── cron/
│   │   └── service.go      # every/cron/at schedules
│   └── heartbeat/
│       └── service.go      # 30-min HEARTBEAT.md check
├── bridge/                 # Copy from nanobot/bridge/ (Node.js, unchanged)
└── workspace/              # Copy from nanobot/workspace/ (default files)
```

---

## Key Dependencies

| Purpose | Library |
|---------|---------|
| CLI | `github.com/spf13/cobra` |
| Readline | `github.com/chzyer/readline` |
| Markdown render | `github.com/charmbracelet/glamour` |
| YAML | `gopkg.in/yaml.v3` |
| Cron | `github.com/robfig/cron/v3` |
| WebSocket | `github.com/gorilla/websocket` |
| Telegram | `github.com/go-telegram-bot-api/telegram-bot-api/v5` |
| Slack | `github.com/slack-go/slack` |
| IMAP | `github.com/emersion/go-imap/v2` |
| HTML extract | `github.com/go-shiori/go-readability` |
| Logging | `log/slog` (stdlib) |
| Parallel tasks | `golang.org/x/sync/errgroup` |

---

## Python → Go Mapping

| Python | Go |
|--------|----|
| `asyncio.Queue` | `chan` (buffered) |
| `async def` / `await` | goroutine + channel |
| `asyncio.gather()` | `errgroup.WithContext()` |
| `asyncio.Lock` | `sync.Mutex` |
| `pydantic.BaseModel` | struct + json tags |
| `loguru` | `log/slog` |
| `litellm` | direct `net/http` to OpenAI-compatible API |
| `croniter` | `github.com/robfig/cron/v3` |

---

## Implementation Phases

### Phase 1 — Foundation
Files: `bus/`, `config/`

- `InboundMessage` / `OutboundMessage` with `SessionKey() string`
- `MessageBus{Inbound, Outbound chan}` (buf 100)
- Config structs mirroring `~/.nanobot/config.json` camelCase exactly
- `MatchProvider()`: explicit prefix → keyword → gateway fallback → standard → skip OAuth
- `migrateConfig()`: handle `exec.restrictToWorkspace` legacy key

### Phase 2 — Session + Memory
Files: `session/manager.go`, `agent/memory.go`

- JSONL: line 1 = `{"_type":"metadata",...,"last_consolidated":N}`, rest = messages
- `json.Encoder` with `SetEscapeHTML(false)` (CJK compat)
- `sync.Map` in-memory cache; safe filename (`:`→`_`)
- `GetHistory(n)` filters to `role,content,tool_calls,tool_call_id,name`
- `Consolidate()` with identical `save_memory` tool schema; per-session mutex

### Phase 3 — Providers
Files: `providers/`

- 18 `ProviderSpec` entries verbatim from Python registry
- `OpenAIProvider`: POST `{apiBase}/chat/completions`; `resolveModel()`, `sanitizeMessages()`, `applyCacheControl()`, JSON-repair fallback
- Anthropic: `anthropic-version` header
- `CodexProvider`: OAuth token, SSE streaming

### Phase 4 — Tools
Files: `tools/`

- `Tool` interface + `Registry.GetDefinitions()` → OpenAI function schema
- `ExecTool`: 9 RE2 deny patterns; `exec.CommandContext`; workspace path check
- `EditFileTool`: similarity hint when `old_text` not found
- `WebSearchTool`: Brave API; `WebFetchTool`: `go-readability`
- `MessageTool`: `SetContext` / `StartTurn` / `WasSentInTurn`
- `SpawnTool` / `CronTool`: context injection per turn
- `MCPToolWrapper`: stdio subprocess + HTTP POST

### Phase 5 — Agent Loop
Files: `agent/`

- `Run()`: `for { select { case msg := <-bus.Inbound: go handleMessage() } }`
- `processMessage()`: system msgs → slash cmds → build ctx → run loop → save session
- `runAgentLoop()`: LLM → tool calls → append results → loop (max 20)
- `setToolContext()`: inject channel/chatID into Message/Spawn/CronTool before each turn
- `stripThink()`: remove `<think>…</think>` (DeepSeek, Kimi)
- MCP lazy connect via `atomic.Bool`
- `ProcessDirect()`: for CLI + cron (no bus)

### Phase 6 — CLI
Files: `cmd/`, `main.go`

- `onboard`: create config + workspace templates (identical content to Python)
- `agent`: readline loop + `-m` single-message + Glamour markdown
- `gateway`: `errgroup` for agent + channels + cron + heartbeat; SIGINT graceful shutdown
- `status`: table of config/model/providers
- `cron`: list/add/remove/enable/run with identical flags
- Embed: `//go:embed workspace/* internal/skills/*/SKILL.md`

### Phase 7 — Channels
Files: `channels/`
Priority: Telegram → WhatsApp → Discord → Slack → Email → Feishu → DingTalk → Mochat → QQ

- **Telegram**: markdown→HTML two-pass converter; split at 4000 (`rfind('\n')` → `rfind(' ')` → hard cut); typing goroutine
- **WhatsApp**: WS client to `ws://127.0.0.1:3001`; auth handshake; 5s reconnect
- **Discord**: raw Gateway WS; opcodes 0/1/2/10/11; split at 2000; REST sends
- **Slack**: `slack-go` Socket Mode; thread replies; DM/group policy
- **Email**: IMAP poll + SMTP; `consent_granted` gate; UID dedup; Re: subject tracking
- Allowlist: `"id|username"` pipe format (Telegram); plain ID (others)

### Phase 8 — Cron + Heartbeat
Files: `cron/service.go`, `heartbeat/service.go`

- `CronJob` camelCase JSON: `nextRunAtMs`, `lastRunAtMs`, `atMs`, `everyMs`
- Schedules: `every`→Ticker, `cron`→robfig+TZ, `at`→AfterFunc one-shot
- `isHeartbeatEmpty()`: skip empty lines, `#` headers, `<!--` comments, `- [ ]` / `- [x]` boxes

---

## Non-Negotiable Compatibility Contracts

| Item | Requirement |
|------|-------------|
| Session JSONL | `_type:"metadata"` line 1; `last_consolidated` int; append-only messages |
| Config keys | camelCase; same field names as existing `~/.nanobot/config.json` |
| Tool schemas | Byte-identical JSON parameter definitions (LLM sees same functions) |
| `last_consolidated` | Slice `messages[N : len-keepCount]` for incremental consolidation |
| Deny patterns | 9 RE2 patterns; same blocked commands as Python |
| Message splitting | Telegram 4000 / Discord 2000; prefer line-break splits |
| `save_memory` schema | `history_entry` + `memory_update` required fields, identical descriptions |
| `jobs.json` | camelCase keys; same schema for Python↔Go job file compatibility |

---

## Verification Checklist

- [ ] `go build -o crystaldolphin ./main.go` — single binary, no errors
- [ ] `go test ./...` — all unit tests pass
- [ ] `./crystaldolphin onboard` — creates correct `~/.nanobot/config.json`
- [ ] `./crystaldolphin status` — reads Python-generated config correctly
- [ ] `./crystaldolphin agent -m "hello"` — returns LLM response
- [ ] Session written by Go readable by Python (and vice versa)
- [ ] Tool `GetDefinitions()` matches Python golden JSON schemas
- [ ] Cross-compile: linux/amd64, darwin/arm64, windows/amd64
