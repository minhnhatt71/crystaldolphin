package services

import "github.com/crystaldolphin/crystaldolphin/internal/modeling/session"

// MemoryCompactor schedules background memory consolidation for a session.
// Defined locally to avoid importing the legacy schema package.
type MemoryCompactor interface {
	Schedule(key string, sess session.Session)
}

// MemoryReader provides read access to the agent's long-term memory.
// Defined locally so services do not depend on the memory.Store infrastructure interface.
type MemoryReader interface {
	ReadLongTermMemory() (string, error)
}
