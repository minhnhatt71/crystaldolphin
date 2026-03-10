package session

// Session represents a single conversation's state.
// Concrete implementations (JSONL-backed, in-memory, etc.) live outside modeling/.
type Session interface {
	// Record appends a SessionEntry and returns the session for chaining.
	Record(entry SessionEntry) Session

	// History returns the last max entries. Passing max <= 0 returns all.
	History(max int) []SessionEntry

	// Clear resets the session, discarding all entries and the compaction pointer.
	Clear()

	// Len returns the total number of stored entries.
	Len() int

	// Compact advances the compaction pointer after a successful run.
	// archive=true clears all entries; false trims to the keepCount tail.
	Compact(archive bool, keepCount int)
}
