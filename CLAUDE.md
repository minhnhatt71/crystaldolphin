# crystaldolphin — Contributor Guide

## Architecture

The codebase is organized into four layers, from innermost to outermost:

| Layer | Package | Responsibility |
| ----- | ------- | -------------- |
| Domain | `internal/modeling/` | Interfaces and value types only — no I/O |
| Agent | `internal/agent/` | LLM ↔ tool iteration loop; central to the application |
| Worker | `internal/workers/` | Settings, skills, and prompt building for a specific turn role |
| Service | `internal/services/` | Session, memory, and compaction orchestration |

See [.claude/rules/modeling-architecture.md](.claude/rules/modeling-architecture.md) for the domain layer design: `internal/modeling/` package map, DDD rules, `Agent`/`Session`/`Memory` interfaces, and the concrete `internal/agent/` implementation.

See [.claude/rules/service-worker-architecture.md](.claude/rules/service-worker-architecture.md) for the Service/Worker/Agent layer design: responsibilities, data flow, dependency direction, and field ownership rules.

## Coding Conventions

### Value types (structs in `internal/modeling/`)

- All fields are **private** (lowercase).
- Expose a constructor for required fields: `NewFoo(requiredA, requiredB)`.
- Expose `With*` copy-and-return helpers for optional fields: `func (f Foo) WithBar(bar string) Foo`.
- Expose read-only **accessor methods** for every field: `func (f Foo) Bar() string`.
- Never access struct fields directly from outside the package.

## DevOps

See [.claude/rules/devops.md](.claude/rules/devops.md) for Docker setup, Compose services, cross-compilation, gateway port, and runtime data directory.

---

## Build & Run

All common tasks are covered by the Makefile.

| Command            | Description                                        |
| ------------------ | -------------------------------------------------- |
| `make`             | Build Go binary + WhatsApp bridge                  |
| `make build`       | Compile the Go binary (`./crystaldolphin`)         |
| `make run`         | Build then run the binary                          |
| `make dev`         | Run with `go run` (no compile step)                |
| `make bridge`      | Install npm deps and compile the TypeScript bridge |
| `make bridge-dev`  | Run the bridge in dev mode (`tsc && node`)         |
| `make docker`      | `docker compose build`                             |
| `make docker-up`   | Start services in the background                   |
| `make docker-down` | Stop services                                      |
| `make test`        | Run all Go tests                                   |
| `make tidy`        | `go mod tidy`                                      |
| `make clean`       | Remove binary and `bridge/dist/`                   |
