package session

import (
	"sync"
	"time"

	"log/slog"

	"github.com/crystaldolphin/crystaldolphin/internal/schema"
)

// SessionImpl holds one conversation's messages and metadata.
type SessionImpl struct {
	Key              string
	Messages         schema.Messages
	CreatedAt        time.Time
	UpdatedAt        time.Time
	Metadata         map[string]any
	lastConsolidated int // number of messages already consolidated to MEMORY.md/HISTORY.md

	mu sync.Mutex
}

// newSession constructs a Session with all fields set, including the unexported
// lastConsolidated counter. Used only by the manager when loading from disk.
func newSession(key string, messages schema.Messages, createdAt, updatedAt time.Time, meta map[string]any, lastConsolidated int) schema.Session {
	return &SessionImpl{
		Key:              key,
		Messages:         messages,
		CreatedAt:        createdAt,
		UpdatedAt:        updatedAt,
		Metadata:         meta,
		lastConsolidated: lastConsolidated,
	}
}

// NewArchivedSession creates a temporary session with pre-populated messages
// and no consolidation history. Used for /new consolidation of the old snapshot.
func NewArchivedSession(key string, messages schema.Messages) schema.Session {
	return &SessionImpl{
		Key:      key,
		Messages: messages,
	}
}

// AddUser appends a user message to the session.
func (s *SessionImpl) AddUser(content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Messages.AddUser(content)
	s.UpdatedAt = time.Now()
}

// AddAssistant appends an assistant message to the session.
func (s *SessionImpl) AddAssistant(content string, toolsUsed []string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c := content
	msg := schema.Message{
		Role:      "assistant",
		Content:   &c,
		ToolsUsed: toolsUsed,
	}

	s.Messages.Add(msg)
	s.UpdatedAt = time.Now()
}

// GetHistory returns the last maxMessages messages for the LLM.
func (s *SessionImpl) GetHistory(maxMessages int) schema.Messages {
	s.mu.Lock()
	defer s.mu.Unlock()

	msgs := s.Messages.Messages
	if maxMessages > 0 && len(msgs) > maxMessages {
		msgs = msgs[len(msgs)-maxMessages:]
	}

	out := schema.NewMessages()
	out.Messages = append(out.Messages, msgs...)
	return out
}

// Len returns the number of messages in the session.
func (s *SessionImpl) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.Messages.Messages)
}

// Clear resets messages and the consolidation pointer.
func (s *SessionImpl) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Messages = schema.NewMessages()
	s.lastConsolidated = 0
	s.UpdatedAt = time.Now()
}

// LastConsolidated returns the consolidation pointer.
// Caller must hold s.mu.
func (s *SessionImpl) LastConsolidated() int {
	return s.lastConsolidated
}

// Consolidate updates the consolidation cursor after a successful run.
// archive=true resets lastConsolidated to 0; false compacts to the keepCount tail.
// Must only be called from the consolidation goroutine (never concurrently).
func (s *SessionImpl) Consolidate(archive bool, keepCount int) {
	if archive {
		s.lastConsolidated = 0
		s.UpdatedAt = time.Now()
		s.Messages = schema.NewMessages()
	} else {
		msgs := s.Messages.Messages
		if keepCount <= 0 || len(msgs) <= keepCount {
			return
		}
		tail := make([]schema.Message, keepCount)
		copy(tail, msgs[len(msgs)-keepCount:])
		s.Messages.Messages = tail
		s.lastConsolidated = 0
		s.UpdatedAt = time.Now()
	}
}

// ConsolidatedMessages returns the slice of messages eligible for consolidation and
// true, or an empty Messages and false when there is nothing to do.
// Must only be called from the consolidation goroutine (never concurrently).
func (s *SessionImpl) ConsolidatedMessages(archive bool, memWindow, keepCount int) (schema.Messages, bool) {
	msgs := s.Messages.Messages
	lastConsolidated := s.lastConsolidated

	if archive {
		slog.Info("memory consolidation (archive_all)", "messages", len(msgs))
		return schema.NewMessages(msgs...), true
	}

	if len(msgs) <= keepCount || len(msgs)-lastConsolidated <= 0 {
		return schema.NewMessages(), false
	}

	end := len(msgs) - keepCount
	if end <= lastConsolidated {
		return schema.NewMessages(), false
	}

	oldMsgs := msgs[lastConsolidated:end]
	if len(oldMsgs) == 0 {
		return schema.NewMessages(), false
	}

	slog.Info("memory consolidation", "to_consolidate", len(oldMsgs), "keep", keepCount)

	return schema.NewMessages(oldMsgs...), true
}
