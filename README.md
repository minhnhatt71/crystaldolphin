# crystaldolphin 🐬

A Go rewrite of [nanobot](https://github.com/HKUDS/nanobot) — an ultra-lightweight personal AI assistant. Single static binary, no runtime dependencies, full behavioral parity with the Python original.

[![Go](https://img.shields.io/badge/go-1.25+-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)

## Why Go?

| | nanobot (Python) | crystaldolphin (Go) |
|---|---|---|
| Distribution | Python runtime + pip | Single binary (~13 MB) |
| Memory | ~80–150 MB | ~20–40 MB |
| Startup | ~1–2 s | ~50 ms |
| Concurrency | asyncio event loop | Native goroutines |
| Cross-compile | ✗ | ✓ (Linux / macOS / Windows) |

Everything else is identical: same `~/.nanobot/config.json` format, same session JSONL files, same tool schemas, same channels.

## Quick Start

```bash
# Build
go build -o crystaldolphin ./main.go

# Initialize
./crystaldolphin onboard

# Edit ~/.nanobot/config.json — add your API key + model
# then:
./crystaldolphin agent
```

Or download a pre-built binary from [Releases](https://github.com/crystaldolphin/crystaldolphin/releases).

## Install from Source

```bash
git clone https://github.com/crystaldolphin/crystaldolphin.git
cd crystaldolphin
go build -ldflags="-s -w" -o crystaldolphin ./main.go
```

Requires Go 1.25+. No other runtime needed.

## Configuration

Same file as nanobot: `~/.nanobot/config.json`

### Minimal config

```json
{
  "providers": {
    "openrouter": {
      "apiKey": "sk-or-v1-xxx"
    }
  },
  "agents": {
    "defaults": {
      "model": "anthropic/claude-opus-4-5"
    }
  }
}
```

Existing nanobot configs work without any changes.

### Supported providers

| Key | Description |
|-----|-------------|
| `openrouter` | OpenRouter (all models, recommended) |
| `anthropic` | Claude direct |
| `openai` | GPT direct |
| `deepseek` | DeepSeek |
| `groq` | Groq + Whisper voice transcription |
| `gemini` | Gemini |
| `minimax` | MiniMax |
| `aihubmix` | AiHubMix gateway |
| `siliconflow` | SiliconFlow |
| `volcengine` | VolcEngine |
| `dashscope` | Qwen / DashScope |
| `moonshot` | Moonshot / Kimi |
| `zhipu` | Zhipu GLM |
| `vllm` | Any local OpenAI-compatible server |
| `custom` | Any OpenAI-compatible endpoint |
| `openai_codex` | Codex (OAuth, requires `provider login`) |
| `github_copilot` | GitHub Copilot (OAuth, requires `provider login`) |

## CLI Reference

| Command | Description |
|---------|-------------|
| `crystaldolphin onboard` | Create config & workspace |
| `crystaldolphin agent` | Interactive chat |
| `crystaldolphin agent -m "..."` | Single message mode |
| `crystaldolphin agent --markdown` | Render Markdown output |
| `crystaldolphin gateway` | Start the multi-channel gateway |
| `crystaldolphin status` | Show config, model, provider status |
| `crystaldolphin channels status` | Show channel configs |
| `crystaldolphin channels login` | Link WhatsApp via QR code |
| `crystaldolphin provider login openai-codex` | OAuth login for Codex |
| `crystaldolphin provider login github-copilot` | OAuth login for Copilot |
| `crystaldolphin cron list` | List scheduled jobs |
| `crystaldolphin cron add ...` | Add a scheduled job |
| `crystaldolphin cron remove <id>` | Remove a job |
| `crystaldolphin cron run <id>` | Run a job manually |

Interactive mode exits: `exit`, `quit`, `:q`, or Ctrl+D.

### cron add flags

| Flag | Description |
|------|-------------|
| `-n TEXT` | Job name (required) |
| `-m TEXT` | Message to send agent (required) |
| `-e N` | Run every N seconds |
| `-c EXPR` | Cron expression (e.g. `"0 9 * * *"`) |
| `--at ISO` | Run once at ISO datetime |
| `--tz TZ` | Timezone for cron (e.g. `Asia/Shanghai`) |
| `-d` | Deliver response to a channel |
| `--to ID` | Recipient ID |
| `--channel CH` | Channel name |

## Chat Channels

Enable any channel in `~/.nanobot/config.json`:

```json
{
  "channels": {
    "telegram": {
      "enabled": true,
      "token": "YOUR_BOT_TOKEN",
      "allowFrom": ["YOUR_USER_ID"]
    }
  }
}
```

Then run: `crystaldolphin gateway`

### Telegram

Get a token from [@BotFather](https://t.me/BotFather).

```json
"telegram": {
  "enabled": true,
  "token": "YOUR_BOT_TOKEN",
  "allowFrom": ["YOUR_USER_ID"]
}
```

### Discord

Create a bot at [discord.com/developers](https://discord.com/developers/applications). Enable **Message Content Intent**.

```json
"discord": {
  "enabled": true,
  "token": "YOUR_BOT_TOKEN",
  "allowFrom": ["YOUR_USER_ID"]
}
```

### WhatsApp

Requires Node.js ≥18 (included in Docker image).

```bash
crystaldolphin channels login   # scan QR code
```

```json
"whatsapp": {
  "enabled": true,
  "allowFrom": ["+1234567890"]
}
```

### Feishu (飞书)

Uses WebSocket long connection — no public IP needed.

```json
"feishu": {
  "enabled": true,
  "appId": "cli_xxx",
  "appSecret": "xxx"
}
```

### DingTalk (钉钉)

Uses Stream Mode — no public IP needed.

```json
"dingtalk": {
  "enabled": true,
  "clientId": "YOUR_APP_KEY",
  "clientSecret": "YOUR_APP_SECRET"
}
```

### Slack

Uses Socket Mode — no public URL needed. Requires a bot token (`xoxb-...`) and app-level token (`xapp-...`).

```json
"slack": {
  "enabled": true,
  "botToken": "xoxb-...",
  "appToken": "xapp-...",
  "groupPolicy": "mention"
}
```

`groupPolicy`: `"mention"` (respond only when @mentioned), `"open"` (all messages), `"allowlist"`.

### Email

Polls IMAP for incoming mail, replies via SMTP. Must set `consentGranted: true`.

```json
"email": {
  "enabled": true,
  "consentGranted": true,
  "imapHost": "imap.gmail.com",
  "imapPort": 993,
  "imapUsername": "bot@gmail.com",
  "imapPassword": "app-password",
  "smtpHost": "smtp.gmail.com",
  "smtpPort": 587,
  "smtpUsername": "bot@gmail.com",
  "smtpPassword": "app-password",
  "fromAddress": "bot@gmail.com",
  "allowFrom": ["you@gmail.com"]
}
```

### QQ

Uses QQ bot gateway WebSocket — no public IP needed.

```json
"qq": {
  "enabled": true,
  "appId": "YOUR_APP_ID",
  "secret": "YOUR_APP_SECRET"
}
```

### Mochat

HTTP polling.

```json
"mochat": {
  "enabled": true,
  "baseUrl": "https://mochat.io",
  "clawToken": "claw_xxx"
}
```

## MCP (Model Context Protocol)

```json
{
  "tools": {
    "mcpServers": {
      "filesystem": {
        "command": "npx",
        "args": ["-y", "@modelcontextprotocol/server-filesystem", "/path/to/dir"]
      },
      "remote": {
        "url": "https://example.com/mcp/",
        "headers": { "Authorization": "Bearer token" }
      }
    }
  }
}
```

Stdio and HTTP transports are supported. Config format is compatible with Claude Desktop / Cursor.

## Security

| Option | Default | Description |
|--------|---------|-------------|
| `tools.restrictToWorkspace` | `false` | Sandbox all file/shell tools to workspace directory |
| `channels.*.allowFrom` | `[]` (all) | Allowlist of user IDs per channel |

## Docker

### Docker Compose

```bash
# First-time setup
docker compose run --rm crystaldolphin-cli onboard
vim ~/.nanobot/config.json   # add API keys

# Start gateway
docker compose up -d crystaldolphin-gateway

# CLI commands
docker compose run --rm crystaldolphin-cli agent -m "Hello!"
docker compose run --rm crystaldolphin-cli status

# Logs & shutdown
docker compose logs -f crystaldolphin-gateway
docker compose down
```

### Docker (standalone)

```bash
docker build -t crystaldolphin .

docker run -v ~/.nanobot:/root/.nanobot --rm crystaldolphin onboard
docker run -v ~/.nanobot:/root/.nanobot -p 18790:18790 crystaldolphin gateway
docker run -v ~/.nanobot:/root/.nanobot --rm crystaldolphin agent -m "Hello!"
```

## Cross-Compilation

```bash
GOOS=linux   GOARCH=amd64 go build -ldflags="-s -w" -o crystaldolphin-linux-amd64 ./main.go
GOOS=darwin  GOARCH=arm64 go build -ldflags="-s -w" -o crystaldolphin-darwin-arm64 ./main.go
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o crystaldolphin.exe          ./main.go
```

## Project Structure

```
crystaldolphin/
├── main.go
├── cmd/                    # CLI commands (cobra)
│   ├── root.go
│   ├── onboard.go
│   ├── agent.go
│   ├── gateway.go
│   ├── status.go
│   ├── cron.go
│   └── channels.go
├── internal/
│   ├── agent/              # Core agent loop, context, memory, skills, subagent
│   ├── tools/              # Shell, filesystem, web, MCP, spawn, cron, message
│   ├── providers/          # LLM providers (OpenAI-compatible + Codex OAuth)
│   ├── channels/           # Telegram, Discord, WhatsApp, Slack, Feishu, DingTalk,
│   │                       #   Email, Mochat, QQ + manager
│   ├── bus/                # InboundMessage / OutboundMessage + MessageBus
│   ├── session/            # JSONL session storage
│   ├── cron/               # Scheduled job runner
│   ├── heartbeat/          # 30-min proactive wake-up
│   └── config/             # Config schema + loader
├── bridge/                 # WhatsApp Node.js bridge (unchanged from nanobot)
├── workspace/              # Default workspace files (AGENTS.md, SOUL.md, etc.)
├── Dockerfile
└── docker-compose.yml
```

## Compatibility with nanobot

- `~/.nanobot/config.json` — identical JSON keys; existing configs work without changes
- `~/.nanobot/sessions/` — identical JSONL format; sessions are interchangeable
- `~/.nanobot/cron/jobs.json` — identical schema
- `workspace/` files — identical (AGENTS.md, SOUL.md, MEMORY.md, HISTORY.md, HEARTBEAT.md)
- Tool JSON schemas — identical function definitions seen by the LLM
- WhatsApp bridge — same Node.js bridge code, unchanged

## License

MIT
