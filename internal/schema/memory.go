package schema

import "context"

// ConsolidatableSession is the subset of session.Session required by
// MemoryStore.Consolidate. Defined here to avoid an import cycle
// (session imports schema, so schema cannot import session).
type ConsolidatableSession interface {
	Lock()
	Unlock()
	CopyMessages() Messages    // returns a snapshot of the current message list
	LastConsolidated() int     // returns the current LastConsolidated pointer
	SetLastConsolidated(n int) // updates LastConsolidated (caller must hold lock)
	Compact(keepCount int)
}

// SessionSaver persists a session after consolidation advances its pointer.
type SessionSaver interface {
	SaveConsolidated(s ConsolidatableSession) error
}

// MemoryStore manages long-term memory and history for the agent.
type MemoryStore interface {
	ReadLongTerm() string
	WriteLongTerm(content string) error
	AppendHistory(entry string) error
	GetMemoryContext() string
	Consolidate(ctx context.Context,
		s ConsolidatableSession, saver SessionSaver, provider LLMProvider,
		model string, archiveAll bool, memoryWindow int,
	) error
}
