# Modeling Architecture

## Overview

`internal/modeling/` is the **domain layer** of crystaldolphin. It defines what the agent *is* — its interfaces, value objects, and domain contracts — with no dependency on I/O, frameworks, or infrastructure.

Everything outside `modeling/` is the **implementation layer**: concrete structs that satisfy the domain interfaces (e.g. `internal/agent/`, `internal/agentlegacy/`, `internal/session/`).

```
internal/modeling/          ← domain layer (interfaces + value types only)
    agent/                  ← Agent interface; ToolHandler/Registry interfaces; ChatOption
    llmprovider/            ← LLMProvider interface; Settings, LLMResponse value types
    prompt/                 ← Prompt, Prompts value types; builder helpers
    memory/                 ← LongTerm, History, HistoryEntry value types; Store interface
    session/                ← Session interface; SessionEntry value type; Store interface
    skills/                 ← Loader interface
    channel/                ← Channel interface; Message value type; Manager interface

internal/agent/             ← concrete Agent implementation (infrastructure)
    agent.go                ← Agent struct implementing modeling/agent.Agent
```

---

## Domain Rules

Files inside `internal/modeling/` must contain **only**:

- Interfaces
- Value types / structs with pure data fields and accessors
- Constructor / builder helper functions (pure, no I/O)

Files inside `internal/modeling/` must **never** contain:

- Structs holding live dependencies (provider, registry, buses, goroutines)
- File I/O, HTTP calls, database access, or any OS interaction
- Concurrency primitives (goroutines, channels, mutexes)

---

## Package Map

### `modeling/agent/`

The centrepiece. Everything else in `modeling/` exists to support this package.

| File | Contents |
|---|---|
| `agent.go` | `Agent` interface — `Chat(ctx, prompts, ...ChatOption) (string, error)` |
| `registry.go` | `ToolHandler` interface; `ToolHandlerRegistry` interface |
| `chat_option.go` | `ChatOption` functional option; `ChatArgs` value type; `WithPrompt`, `RetrieveChatArgs` |
| `builder.go` | Reserved for factory helpers (currently empty) |

**`Agent` interface:**
```go
type Agent interface {
    Chat(ctx context.Context, prompts prompt.Prompts, opts ...ChatOption) (string, error)
}
```

**`ToolHandler` / `ToolHandlerRegistry` interfaces:**
```go
type ToolHandler interface {
    Execute(ctx context.Context, args map[string]any) (string, error)
}

type ToolHandlerRegistry interface {
    Get(name string) ToolHandler
}
```

---

### `modeling/llmprovider/`

LLM communication contract and its supporting value types.

| File | Contents |
|---|---|
| `provider.go` | `LLMProvider` interface — `Chat(ctx, prompt.Prompts, Settings) (LLMResponse, error)` |
| `settings.go` | `Settings` value type — model, maxIterations, temperature, maxTokens, memoryWindow |
| `mapping.go` | `Tool` and `Tools` value types (implement `prompt.Tool` / `prompt.Tools`); `LLMResponse` |
| `utils.go` | `StripThink(s string) string`; `Truncate(s string, n int) string` |

---

### `modeling/prompt/`

The conversation representation passed to `LLMProvider.Chat` and `Agent.Chat`.

| File | Contents |
|---|---|
| `prompt.go` | `Prompt` value type (role, content, tools, reasoning); `Tool` interface; `Tools` interface |
| `prompts.go` | `Prompts` value type (ordered list of `Prompt`); `Add`, `Get`, `NewList` |
| `builder.go` | Pure constructors: `New`, `NewUser`, `NewAssistant`, `NewToolCall`, `NewToolCalls`; option helpers `WithTool`, `WithTools`, `WithReasoning` |

---

### `modeling/memory/`

Value types and storage contract for the agent's persistent memory.

| File | Contents |
|---|---|
| `memory.go` | `LongTerm` value type — wraps MEMORY.md content; `NewLongTerm`, `Content`, `IsEmpty` |
| `history.go` | `History` value type (list of `HistoryEntry`); `HistoryEntry` value type; constructors and accessors |
| `store.go` | `Store` interface — `ReadLongterm`, `WriteLongterm`, `ReadHistory`, `WriteHistory` |

---

### `modeling/session/`

Value types and storage contract for per-conversation history.

| File | Contents |
|---|---|
| `entry.go` | `SessionEntry` value type — single message row (role, content, toolsUsed, toolCallID, timestamp); `Role` type + constants; four factories: `NewUserEntry`, `NewAssistantEntry`, `NewToolEntry`, `NewSystemEntry` |
| `session.go` | `Session` interface — `Record`, `History`, `Clear`, `Len`, `Compact` |
| `store.go` | `Store` interface — `GetOrCreate`, `Save`, `Invalidate`, `SaveCompacted` |

**`Session` interface:**
```go
type Session interface {
    Record(entry SessionEntry) Session
    History(max int) []SessionEntry
    Clear()
    Len() int
    Compact(archive bool, keepCount int)
}
```

**`SessionEntry` factories:**
```go
NewUserEntry(content string) SessionEntry
NewAssistantEntry(content string, toolsUsed []string) SessionEntry
NewToolEntry(toolCallID, content string) SessionEntry
NewSystemEntry(content string) SessionEntry
```

---

### `modeling/skills/`

Contract for loading agent skill definitions from the workspace.

| File | Contents |
|---|---|
| `loader.go` | `Loader` interface — `GetAlwaysSkills`, `LoadSkillsForContext`, `BuildSkillsSummary` |

---

### `modeling/channel/`

Domain contract for chat-platform adapters and the manager that routes messages between them and the agent.

| File | Contents |
|---|---|
| `channel.go` | `Channel` interface — `Name`, `Start`, `Send`; `Message` value type with private fields + accessors |
| `manager.go` | `Manager` interface — `Register`, `Start`, `Send` |

**`Channel` interface:**
```go
type Channel interface {
    Name() string
    Start(ctx context.Context) error
    Send(ctx context.Context, msg Message) error
}
```

**`Manager` interface:**
```go
type Manager interface {
    Register(ch Channel)
    Start(ctx context.Context) error
    Send(ctx context.Context, channelName string, msg Message) error
}
```

Concrete implementations live in `internal/channels/`.

---

## Concrete Implementation: `internal/agent/`

`internal/agent/agent.go` contains the only current concrete implementation of `modeling/agent.Agent`.

| Type | Role |
|---|---|
| `Agent` struct | Holds `llmprovider.Settings`, `llmprovider.LLMProvider`, `modeling/agent.ToolHandlerRegistry` |
| `New(settings, provider, registry)` | Constructor; returns `*Agent` (satisfies `modeling/agent.Agent`) |
| `Chat(ctx, prompts, ...ChatOption)` | LLM ↔ tool iteration loop; runs up to `settings.MaxIterations()` turns |

**Loop behaviour:**
1. Call `provider.Chat(ctx, prompts, settings)` to get `LLMResponse`
2. If no tool calls → strip `<think>` blocks from content, return
3. Append assistant turn with tool calls to `prompts`
4. Execute each tool via `registry.Get(name).Execute(ctx, args)`
5. Append each tool result to `prompts`
6. Repeat from step 1

---

## Dependency Direction

```
internal/agent/         → internal/modeling/agent/
                        → internal/modeling/llmprovider/
                        → internal/modeling/prompt/

internal/modeling/agent/    → internal/modeling/prompt/
internal/modeling/llmprovider/ → internal/modeling/prompt/
internal/modeling/session/  → (none — no modeling/ imports)
internal/modeling/memory/   → (none — no modeling/ imports)
internal/modeling/skills/   → (none — no modeling/ imports)
```

`modeling/` packages must never import each other in a cycle. `session`, `memory`, and `skills` are leaf packages with no intra-modeling imports.

---

## Where to Look for Common Tasks

| Task | File(s) to touch |
|---|---|
| Change what `Agent.Chat` does | `internal/agent/agent.go` |
| Add a new `ChatOption` | `internal/modeling/agent/chat_option.go` |
| Add a tool to the registry | Implement `modeling/agent.ToolHandler`; register in the DI container |
| Change LLM request parameters | `internal/modeling/llmprovider/settings.go` |
| Change conversation format | `internal/modeling/prompt/` |
| Change session storage schema | `internal/modeling/session/entry.go` + concrete impl |
| Change memory format | `internal/modeling/memory/memory.go` or `history.go` + concrete impl |
| Add a new skill loader method | `internal/modeling/skills/loader.go` + concrete impl |
