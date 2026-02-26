package schema

import "context"

// Session is the subset of session.Session required by
// MemoryStore.Consolidate. Defined here to avoid an import cycle
// (session imports schema, so schema cannot import session).
type Session interface {
	// ConsolidatedMessages returns the slice of messages eligible for consolidation and
	// true, or an empty Messages and false when there is nothing to do.
	// Must only be called from the consolidation goroutine (never concurrently).
	ConsolidatedMessages(archive bool, memWindow, keepCount int) (Messages, bool)

	// LastConsolidated returns the consolidation pointer.
	LastConsolidated() int // returns the current LastConsolidated pointer

	// Consolidate updates the consolidation cursor after a successful run.
	// archive=true resets lastConsolidated to 0; false compacts to the keepCount tail.
	// Must only be called from the consolidation goroutine (never concurrently).
	Consolidate(archive bool, keepCount int)
}

// SessionSaver persists a session after consolidation advances its pointer.
type SessionSaver interface {
	SaveConsolidated(s Session) error
}

// MemoryStore manages long-term memory and history for the agent.
// It is responsible only for storage I/O; consolidation logic lives in Consolidator.
type MemoryStore interface {
	ReadLongTerm() string
	WriteLongTerm(content string) error
	AppendHistory(entry string) error
	GetMemoryContext() string
}

// MemoryConsolidator orchestrates memory consolidation: it selects old messages,
// calls the LLM to summarise them, and persists the result via a MemoryStore.
type MemoryConsolidator interface {
	Consolidate(ctx context.Context, s Session, archiveAll bool, memoryWindow int) error
}

// Memory is the result of a consolidation selection: the messages to process.
type Memory struct {
	Messages []Message
}

// NewMemory constructs a consolidation result.
func NewMemory(msgs []Message) Memory {
	return Memory{Messages: msgs}
}
