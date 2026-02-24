# crystaldolphin — Contributor Guide

## Architecture

See [.claude/rules/architecture.md](.claude/rules/architecture.md) for the full package map, key interfaces, task-to-file lookup table, and compatibility contracts.

See [.claude/rules/agent-architecture.md](.claude/rules/agent-architecture.md) for data flow, bus message types, and direct processing.

## DevOps

See [.claude/rules/devops.md](.claude/rules/devops.md) for Docker setup, Compose services, cross-compilation, gateway port, and runtime data directory.

---

## Build & Run

All common tasks are covered by the Makefile.

| Command | Description |
|---|---|
| `make` | Build Go binary + WhatsApp bridge |
| `make build` | Compile the Go binary (`./crystaldolphin`) |
| `make run` | Build then run the binary |
| `make dev` | Run with `go run` (no compile step) |
| `make bridge` | Install npm deps and compile the TypeScript bridge |
| `make bridge-dev` | Run the bridge in dev mode (`tsc && node`) |
| `make docker` | `docker compose build` |
| `make docker-up` | Start services in the background |
| `make docker-down` | Stop services |
| `make test` | Run all Go tests |
| `make tidy` | `go mod tidy` |
| `make clean` | Remove binary and `bridge/dist/` |
