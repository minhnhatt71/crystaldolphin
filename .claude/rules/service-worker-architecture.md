# Service & Worker Architecture

## Layer Map

```
┌─────────────────────────────────────────────────────┐
│  Gateway / CLI  (cmd/, agentlegacy/)                │  ← bus wiring, channel routing
├─────────────────────────────────────────────────────┤
│  Service  (internal/services/)                      │  ← session, memory, compaction
├─────────────────────────────────────────────────────┤
│  Worker   (internal/workers/)                       │  ← settings, skills, prompt building
├─────────────────────────────────────────────────────┤
│  Agent    (internal/agent/)                         │  ← LLM ↔ tool loop
├─────────────────────────────────────────────────────┤
│  Domain   (internal/modeling/)                      │  ← interfaces, value types
└─────────────────────────────────────────────────────┘
```

---

## Responsibilities

### Agent — `internal/agent/`

The **central component**. Owns the LLM ↔ tool iteration loop.

- Accepts a `prompt.Prompts` list and `ChatOption`s
- Calls `LLMProvider.Chat` repeatedly until the LLM stops issuing tool calls
- Executes each tool call through `ToolHandlerRegistry`
- Returns the final text response

The agent is **pure**: it knows nothing about sessions, users, channels, or memory. It only knows prompts and tools.

### Worker — `internal/workers/`

Sits **above the agent**. Specialises a turn for a particular role (user chat, background task).

Responsibilities:
- Owns `settings` (model, temperature, max tokens, max iterations)
- Owns `skillsLoader` — decides which skills to inject into the system prompt
- Builds the full system prompt from pre-resolved inputs (memory content, skills, channel context)
- Converts session history entries into `prompt.Prompts`
- Calls `agent.Chat` with the assembled prompts
- Returns `WorkerResult{Content, ToolsUsed}`

**Does not** touch stores, sessions, or the bus. All infrastructure data (long-term memory string, history entries) arrives pre-resolved in `WorkerRequest`.

| Type | File | Purpose |
|---|---|---|
| `WorkerRequest` | `chat_worker.go` | Input: channel, content, history, resolved memory |
| `WorkerResult` | `chat_worker.go` | Output: response text, tools used |
| `Worker` | `chat_worker.go` | Interface: `Process(ctx, WorkerRequest) (WorkerResult, error)` |
| `ChatWorker` | `chat_worker.go` | Full system prompt (memory + skills + session context) |
| `TaskWorker` | `task_worker.go` | Minimal prompt (no memory/skills); for background tasks |
| `ChatWorkerBuilder` | `worker_builder.go` | Fluent builder: agent + settings + skillsLoader |

### Service — `internal/services/`

Sits **above workers**. Orchestrates all infrastructure concerns for a complete request turn.

Responsibilities:
- Load/create the session (`session.Store`)
- Schedule background memory compaction (`MemoryCompactor`)
- Read long-term memory (`MemoryReader`) and pass it to the worker
- Build `WorkerRequest` and call the appropriate `Worker`
- Record user and assistant entries into the session
- Persist the updated session (`session.Store.Save`)
- Return `ServiceResponse` pointing to the correct channel and chat

**Does not** build prompts or decide which skills to use — that is the worker's job.

| Type | File | Purpose |
|---|---|---|
| `AgentService` | `agent_service.go` | User-facing turns: session + memory + ChatWorker |
| `TaskService` | `task_service.go` | Background task turns: parse routing key + TaskWorker |
| `MemoryCompactor` | `compactor.go` | Local interface — schedule background compaction |
| `MemoryReader` | `compactor.go` | Local interface — read long-term memory string |

Both `AgentService` and `TaskService` satisfy `contractagent.AgentService` — a dispatcher can call `.Handle()` on either without type-switching.

---

## Data Flow

```
Inbound request (bus message or CLI call)
    │
    ▼
AgentService.Handle()  or  TaskService.Handle()
    ├── session.Store.GetOrCreate(key)        ← load history
    ├── MemoryCompactor.Schedule(key, sess)   ← kick off background compaction
    ├── MemoryReader.ReadLongTermMemory()     ← resolve memory string
    ├── build WorkerRequest{..., History, LongTermMemory}
    │
    ▼
Worker.Process(ctx, WorkerRequest)
    ├── build system prompt (memory + skills + context)
    ├── convert history → prompt.Prompts
    ├── append user message
    │
    ▼
Agent.Chat(ctx, prompts, ...ChatOption)
    ├── LLMProvider.Chat()
    ├── ToolHandlerRegistry.Execute() × N
    └── return final text
    │
    ▼  (back in Worker)
WorkerResult{Content, ToolsUsed}
    │
    ▼  (back in Service)
    ├── sess.Record(UserEntry).Record(AssistantEntry)
    ├── session.Store.Save(sess)
    └── ServiceResponse{content, channel, chatID}
    │
    ▼
Gateway / CLI routes ServiceResponse to channel bus
```

---

## Dependency Direction

```
internal/services/  →  workers.Worker          (calls)
                    →  session.Store            (infrastructure)
                    →  MemoryReader             (local interface)
                    →  MemoryCompactor          (local interface)
                    →  modeling/agent.AgentService  (satisfies)
                    →  modeling/agent.ServiceRequest / ServiceResponse

internal/workers/   →  modeling/agent.Agent    (calls)
                    →  modeling/llmprovider.Settings
                    →  modeling/skills.Loader
                    →  modeling/session.SessionEntry  (value type only)
                    →  modeling/prompt/

internal/agent/     →  modeling/agent.Agent    (implements)
                    →  modeling/llmprovider.LLMProvider
                    →  modeling/agent.ToolHandlerRegistry
```

`internal/services/` does **not** import `internal/workers/` types for its public interface — the `Worker` interface is the only coupling point. `internal/workers/` does **not** import `internal/services/` or `internal/bus/`.

---

## Worker Field Ownership

| Field | Layer | Rationale |
|---|---|---|
| `agent` | Worker | Workers drive the LLM loop via Agent |
| `settings` | Worker | Each worker role may use different model/temp/iterations |
| `skillsLoader` | Worker (ChatWorker) | Skill injection is a prompt-building concern |
| `workspace` | Worker (TaskWorker) | Task prompts reference workspace path |
| `sessionStore` | Service | Session persistence is infrastructure |
| `memoryStore` | Service | Memory I/O is infrastructure |
| `compactor` | Service | Compaction scheduling is infrastructure |
| `memoryWindow` | Service | Controls how much history to pass to worker |
