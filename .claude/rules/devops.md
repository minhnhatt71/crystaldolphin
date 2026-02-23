# DevOps

## Docker

The project ships a multi-stage `Dockerfile` and a `docker-compose.yml`.

### Image stages

| Stage | Base | Purpose |
|---|---|---|
| `builder` | `golang:1.25-bookworm` | Compiles the Go binary (`CGO_ENABLED=0`, stripped) |
| final | `debian:bookworm-slim` | Runs the binary + Node.js 20 for the WhatsApp bridge |

The final image installs Node.js 20 via the NodeSource repo, copies the binary and `bridge/`, runs `npm install && npm run build`, then sets `ENTRYPOINT ["./crystaldolphin"]`.

### Compose services

| Service | Profile | Command | Port |
|---|---|---|---|
| `crystaldolphin-gateway` | (default) | `gateway start` | 18790 |
| `crystaldolphin-cli` | `cli` | `status` (overridable) | — |

Both services mount `~/.nanobot` from the host at `/root/.nanobot`.

### Common workflows

```bash
# First-time setup
docker compose run --rm crystaldolphin-cli onboard
vim ~/.nanobot/config.json          # add API keys

# Build image
make docker                         # docker compose build

# Start gateway
make docker-up                      # docker compose up -d

# Ad-hoc CLI commands
docker compose run --rm crystaldolphin-cli agent -m "Hello!"
docker compose run --rm crystaldolphin-cli status

# Logs & teardown
docker compose logs -f crystaldolphin-gateway
make docker-down                    # docker compose down
```

### Standalone Docker (no Compose)

```bash
docker build -t crystaldolphin .
docker run -v ~/.nanobot:/root/.nanobot --rm crystaldolphin onboard
docker run -v ~/.nanobot:/root/.nanobot -p 18790:18790 crystaldolphin gateway start
docker run -v ~/.nanobot:/root/.nanobot --rm crystaldolphin agent -m "Hello!"
```

## Build

All build targets live in the `Makefile` (see CLAUDE.md). Key flags used everywhere:

```
CGO_ENABLED=0               pure Go, no C deps — enables static linking
-ldflags="-s -w"            strip debug info and DWARF → smaller binary
```

## Cross-compilation

```bash
GOOS=linux   GOARCH=amd64 go build -ldflags="-s -w" -o crystaldolphin-linux-amd64 ./main.go
GOOS=darwin  GOARCH=arm64 go build -ldflags="-s -w" -o crystaldolphin-darwin-arm64 ./main.go
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o crystaldolphin.exe          ./main.go
```

No extra setup needed — no CGo, no system libraries.

## Gateway port

The gateway listens on **18790** (TCP). Expose this port when running in Docker or behind a reverse proxy. No inbound port is required for Telegram, Discord, Slack, Feishu, DingTalk, or QQ — all use outbound connections (polling / WebSocket / Stream Mode).

## Runtime data directory

All persistent data lives in `~/.nanobot/` (host) / `/root/.nanobot/` (container):

| Path | Contents |
|---|---|
| `config.json` | Main configuration |
| `sessions/` | JSONL conversation history |
| `cron/jobs.json` | Scheduled jobs |
| `whatsapp/` | Baileys session credentials |
| `gateway.pid` | PID of the running gateway process (written by `gateway start`) |
