package session

// Store manages the lifecycle of Sessions keyed by routing key
// (e.g. "telegram:12345678"). Concrete implementations live outside modeling/.
type Store interface {
	// GetOrCreate returns the existing session for key, or creates a new one.
	GetOrCreate(key string) Session

	// Save persists the session to durable storage.
	Save(sess Session) error

	// Invalidate removes the session from any in-memory cache so the next
	// GetOrCreate will reload from disk (or start fresh).
	Invalidate(key string)

	// SaveCompacted persists only the compaction pointer update after a
	// memory-consolidation run, without rewriting the full message list.
	SaveCompacted(sess Session) error
}
