package schema

import "context"

// ChannelSession is the subset of session.ChannelSession required by
// MemoryStore.Consolidate. Defined here to avoid an import cycle
// (session imports schema, so schema cannot import session).
type ChannelSession interface {
	// Messages returns the full message history of the session, including all tool calls.
	Messages() Messages

	// CompactedMessages returns the slice of messages eligible for consolidation and
	// true, or an empty Messages and false when there is nothing to do.
	// Must only be called from the consolidation goroutine (never concurrently).
	CompactedMessages(archive bool, memWindow, keepCount int) (Messages, bool)

	// LastCompacted returns the consolidation pointer.
	LastCompacted() int // returns the current LastConsolidated pointer

	// Compact updates the consolidation cursor after a successful run.
	// archive=true resets lastConsolidated to 0; false compacts to the keepCount tail.
	// Must only be called from the consolidation goroutine (never concurrently).
	Compact(archive bool, keepCount int)
}

// SessionSaver persists a session after consolidation advances its pointer.
type SessionSaver interface {
	SaveCompacted(s ChannelSession) error
}

// MemoryStore manages long-term memory and history for the agent.
// It is responsible only for storage I/O; consolidation logic lives in MemoryCompactor.
type MemoryStore interface {
	ReadLongTerm() string
	WriteLongTerm(content string) error
	AppendHistory(entry string) error
	GetMemoryContext() string
}

// MemoryCompactor orchestrates memory consolidation: it selects old messages,
// calls the LLM to summarise them, and persists the result via a MemoryStore.
type MemoryCompactor interface {
	Compact(ctx context.Context, s ChannelSession, archiveAll bool) error
	Schedule(key string, sess ChannelSession, archiveAll bool)
}
