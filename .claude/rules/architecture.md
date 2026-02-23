# Architecture

crystaldolphin is a Go rewrite of [nanobot](https://github.com/HKUDS/nanobot). The goal is a single static binary with identical behavior: same config format, session files, tool schemas, and channel UX.

## Package map

```
main.go                         Entry point — calls cmd.Execute()

cmd/                            CLI layer (cobra)
  root.go                       Root command, registers all sub-commands
  onboard.go                    `onboard` — create config & workspace
  agent.go                      `agent` — interactive / single-message chat
  gateway.go                    `gateway start|stop|status` — manage the gateway server
  status.go                     `status` — display config / provider health
  cron.go                       `cron list|add|remove|run` — manage scheduled jobs
  channels.go                   `channels status|login` — channel management

internal/bus/                   Message bus (decouples channels from agent)
  events.go                     InboundMessage / OutboundMessage structs
  bus.go                        MessageBus — two buffered channels (cap 100 each)
                                  channels → agent via Inbound
                                  agent → channels via Outbound

internal/config/                Configuration
  schema.go                     All Config structs (camelCase JSON tags, mirrors ~/.nanobot/config.json)
  loader.go                     Load / save config; MatchProvider(); migrateConfig()
  match.go                      Provider matching logic

internal/session/               Persistent session storage
  manager.go                    JSONL files; line 1 = metadata; sync.Map in-memory cache

internal/agent/                 Core agent logic
  loop.go                       Run() consumes bus.Inbound; processMessage(); runAgentLoop() (max 20 iters)
  context.go                    Builds system prompt + message history for each LLM call
  memory.go                     MEMORY.md + HISTORY.md read/write; save_memory consolidation
  skills.go                     SKILL.md loader; injects skill XML into system prompt
  subagent.go                   SpawnTool support — runs a background agent goroutine

internal/tools/                 LLM-callable tools
  registry.go                   Tool interface; Registry.Register/Execute/GetDefinitions()
  shell.go                      exec tool — runs shell commands; 9 RE2 deny patterns
  filesystem.go                 read_file / write_file / edit_file / list_dir
  web.go                        web_search (Brave API) + web_fetch (go-readability)
  message.go                    message tool — routes outbound replies via the bus
  spawn.go                      spawn tool — launches sub-agent goroutines
  cron.go                       cron tool — add / list / remove scheduled jobs
  mcp.go                        MCP client — stdio subprocess + HTTP POST transports

internal/providers/             LLM provider integrations
  registry.go                   18 ProviderSpec entries (base URLs, auth styles)
  provider.go                   LLMProvider interface + LLMResponse type
  openai.go                     OpenAI-compatible HTTP client (covers most providers)
  codex.go                      OpenAI Codex — OAuth token + SSE streaming
  factory.go                    Constructs the right provider from config

internal/channels/              Chat platform integrations
  base.go                       Channel interface; Base struct (allowlist, HandleMessage, splitMessage)
  manager.go                    Starts all enabled channels; routes Outbound messages
  telegram.go                   Polling; markdown→HTML; split at 4000 chars
  discord.go                    Raw Gateway WebSocket; split at 2000 chars
  slack.go                      slack-go Socket Mode; thread replies; group policy
  whatsapp.go                   WebSocket client → local Node.js bridge (port 3001)
  feishu.go                     WebSocket long connection
  dingtalk.go                   Stream Mode
  email.go                      IMAP poll + SMTP; consent gate; UID dedup
  mochat.go                     HTTP polling
  qq.go                         QQ bot Gateway WebSocket

internal/cron/
  service.go                    Scheduler — every (Ticker) / cron (robfig+TZ) / at (AfterFunc)

internal/heartbeat/
  service.go                    30-min proactive wake-up; reads HEARTBEAT.md

bridge/                         Node.js / TypeScript WhatsApp bridge (Baileys)
  src/index.ts                  Entry; WebSocket server on :3001
  src/whatsapp.ts               Baileys session handling
  src/server.ts                 WS message framing

workspace/                      Default workspace files copied on `onboard`
  AGENTS.md / SOUL.md / USER.md / TOOLS.md / HEARTBEAT.md
  memory/MEMORY.md
```

See [agent-architecture.md](agent-architecture.md) for data flow, bus message types, and direct processing.

## Key interfaces

| Interface | File | Purpose |
|---|---|---|
| `Tool` | `internal/tools/registry.go` | All LLM-callable tools |
| `Channel` | `internal/channels/base.go` | All chat platform adapters |
| `LLMProvider` | `internal/providers/provider.go` | All LLM backends |

## Where to look for common tasks

| Task | Files to touch |
|---|---|
| Add a new CLI command | `cmd/<name>.go` + register in `cmd/root.go` |
| Add a new LLM provider | `internal/providers/registry.go` + optionally a new `*.go` if auth differs |
| Add a new chat channel | `internal/channels/<name>.go` (implement `Channel`) + enable in `manager.go` + add config field in `internal/config/schema.go` |
| Add a new agent tool | `internal/tools/<name>.go` (implement `Tool`) + register in `agent/loop.go` |
| Change config schema | `internal/config/schema.go` + `loader.go` if migration needed |
| Change session format | `internal/session/manager.go` (mind compatibility contract below) |
| Change system prompt / context | `internal/agent/context.go` |
| Change memory / consolidation | `internal/agent/memory.go` |
| Change cron scheduling | `internal/cron/service.go` + `internal/tools/cron.go` |

## Compatibility contracts (do not break)

- **Config keys** — camelCase; must match existing `~/.nanobot/config.json` field names exactly.
- **Session JSONL** — line 1 is `{"_type":"metadata",...,"last_consolidated":N}`; remaining lines are messages; append-only.
- **Tool schemas** — `GetDefinitions()` output must be byte-identical to the Python originals (LLM sees same functions).
- **`jobs.json`** — camelCase keys; shared with Python nanobot.
- **Message splitting** — Telegram 4000 chars / Discord 2000 chars; prefer newline → space → hard cut.
