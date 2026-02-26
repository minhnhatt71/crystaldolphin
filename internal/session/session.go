package session

import (
	"sync"
	"time"

	"github.com/crystaldolphin/crystaldolphin/internal/schema"
)

// Session holds one conversation's messages and metadata.
type Session struct {
	Key              string
	Messages         schema.Messages
	CreatedAt        time.Time
	UpdatedAt        time.Time
	Metadata         map[string]any
	LastConsolidated int // number of messages already consolidated to MEMORY.md/HISTORY.md

	mu sync.Mutex
}

// AddUser appends a user message to the session.
func (s *Session) AddUser(content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Messages.AddUser(content)
	s.UpdatedAt = time.Now()
}

// AddAssistant appends an assistant message to the session.
func (s *Session) AddAssistant(content string, toolsUsed []string) {
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
func (s *Session) GetHistory(maxMessages int) schema.Messages {
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
func (s *Session) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.Messages.Messages)
}

// Compact drops messages that have already been consolidated, keeping only the
// tail of length keepCount. LastConsolidated is reset to 0 because the
// retained messages are the new beginning of the in-memory slice.
// Callers must not hold s.mu when calling Compact.
func (s *Session) Compact(keepCount int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	msgs := s.Messages.Messages
	if keepCount <= 0 || len(msgs) <= keepCount {
		return
	}
	tail := make([]schema.Message, keepCount)
	copy(tail, msgs[len(msgs)-keepCount:])
	s.Messages.Messages = tail
	s.LastConsolidated = 0
	s.UpdatedAt = time.Now()
}

// Clear resets messages and the consolidation pointer.
func (s *Session) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Messages = schema.NewMessages()
	s.LastConsolidated = 0
	s.UpdatedAt = time.Now()
}

func (s *Session) Lock()   { s.mu.Lock() }
func (s *Session) Unlock() { s.mu.Unlock() }
