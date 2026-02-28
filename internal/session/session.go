package session

import (
	"sync"
	"time"

	"log/slog"

	"github.com/crystaldolphin/crystaldolphin/internal/schema"
)

// ChannelSessionImpl holds one conversation's messages and metadata.
type ChannelSessionImpl struct {
	Key           string
	Entries       schema.Messages
	CreatedAt     time.Time
	UpdatedAt     time.Time
	Metadata      map[string]any
	lastCompacted int

	mu sync.Mutex
}

// newSession constructs a Session with all fields set, including the unexported
// lastCompacted counter. Used only by the manager when loading from disk.
func newSession(key string, messages schema.Messages, createdAt, updatedAt time.Time, meta map[string]any, lastCompacted int) schema.ChannelSession {
	return &ChannelSessionImpl{
		Key:           key,
		Entries:       messages,
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
		Metadata:      meta,
		lastCompacted: lastCompacted,
	}
}

// NewArchivedSession creates a temporary session with pre-populated messages
// and no consolidation history. Used for /new consolidation of the old snapshot.
func NewArchivedSession(key string, messages schema.Messages) schema.ChannelSession {
	return &ChannelSessionImpl{
		Key:     key,
		Entries: messages,
	}
}

// Messages returns the full message history of the session, including all tool calls.
func (s *ChannelSessionImpl) Messages() schema.Messages {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Entries
}

// AddUser appends a user message to the session.
func (s *ChannelSessionImpl) AddUser(content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Entries.AddUser(content)
	s.UpdatedAt = time.Now()
}

// AddAssistant appends an assistant message to the session.
func (s *ChannelSessionImpl) AddAssistant(content string, toolsUsed []string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c := content
	msg := schema.Message{
		Role:      "assistant",
		Content:   &c,
		ToolsUsed: toolsUsed,
	}

	s.Entries.Add(msg)
	s.UpdatedAt = time.Now()
}

// History returns the last messages for the LLM.
func (s *ChannelSessionImpl) History(maxMessages int) schema.Messages {
	s.mu.Lock()
	defer s.mu.Unlock()

	msgs := s.Entries.Messages
	if maxMessages > 0 && len(msgs) > maxMessages {
		msgs = msgs[len(msgs)-maxMessages:]
	}

	out := schema.NewMessages()
	out.Messages = append(out.Messages, msgs...)
	return out
}

// Len returns the number of messages in the session.
func (s *ChannelSessionImpl) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.Entries.Messages)
}

// Clear resets messages and the consolidation pointer.
func (s *ChannelSessionImpl) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Entries = schema.NewMessages()
	s.lastCompacted = 0
	s.UpdatedAt = time.Now()
}

// LastCompacted returns the consolidation pointer.
// Caller must hold s.mu.
func (s *ChannelSessionImpl) LastCompacted() int {
	return s.lastCompacted
}

// Compact updates the consolidation cursor after a successful run.
// archive=true resets lastConsolidated to 0; false compacts to the keepCount tail.
// Must only be called from the consolidation goroutine (never concurrently).
func (s *ChannelSessionImpl) Compact(archive bool, keepCount int) {
	if archive {
		s.lastCompacted = 0
		s.UpdatedAt = time.Now()
		s.Entries = schema.NewMessages()
	} else {
		msgs := s.Entries.Messages
		if keepCount <= 0 || len(msgs) <= keepCount {
			return
		}
		tail := make([]schema.Message, keepCount)
		copy(tail, msgs[len(msgs)-keepCount:])
		s.Entries.Messages = tail
		s.lastCompacted = 0
		s.UpdatedAt = time.Now()
	}
}

// CompactedMessages returns the slice of messages eligible for consolidation and
// true, or an empty Messages and false when there is nothing to do.
// Must only be called from the consolidation goroutine (never concurrently).
func (s *ChannelSessionImpl) CompactedMessages(archive bool, memWindow, keepCount int) (schema.Messages, bool) {
	msgs := s.Entries.Messages
	lastConsolidated := s.lastCompacted

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
