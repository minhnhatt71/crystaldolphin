// Package session manages per-conversation history stored as JSONL files.
//
// File format (byte-compatible with nanobot Python):
//   Line 1:  {"_type":"metadata","key":"…","created_at":"…","updated_at":"…",
//              "metadata":{…},"last_consolidated":N}
//   Line 2+: one JSON message object per line
//
// Messages are append-only; consolidation only writes to memory files.
package session

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Session holds one conversation's messages and metadata.
type Session struct {
	Key              string
	Messages         []map[string]any
	CreatedAt        time.Time
	UpdatedAt        time.Time
	Metadata         map[string]any
	LastConsolidated int // number of messages already consolidated to MEMORY.md/HISTORY.md

	mu sync.Mutex // guards concurrent reads/writes from the agent loop
}

// AddMessage appends a new message to the session.
// extras are merged into the message object (e.g. tool_calls, tools_used).
func (s *Session) AddMessage(role, content string, extras map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()

	msg := map[string]any{
		"role":      role,
		"content":   content,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	for k, v := range extras {
		msg[k] = v
	}
	s.Messages = append(s.Messages, msg)
	s.UpdatedAt = time.Now()
}

// GetHistory returns the last maxMessages messages in LLM format.
// Only role, content, tool_calls, tool_call_id, and name are included —
// stripping session-only fields like timestamp and tools_used.
func (s *Session) GetHistory(maxMessages int) []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()

	msgs := s.Messages
	if maxMessages > 0 && len(msgs) > maxMessages {
		msgs = msgs[len(msgs)-maxMessages:]
	}

	out := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		entry := map[string]any{
			"role":    m["role"],
			"content": m["content"],
		}
		if entry["content"] == nil {
			entry["content"] = ""
		}
		for _, k := range []string{"tool_calls", "tool_call_id", "name"} {
			if v, ok := m[k]; ok {
				entry[k] = v
			}
		}
		out = append(out, entry)
	}
	return out
}

// Clear resets messages and the consolidation pointer.
func (s *Session) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Messages = nil
	s.LastConsolidated = 0
	s.UpdatedAt = time.Now()
}

// Lock / Unlock expose the mutex so the agent loop can hold it across
// multi-step operations (e.g. append → save atomically).
func (s *Session) Lock()   { s.mu.Lock() }
func (s *Session) Unlock() { s.mu.Unlock() }

// ---------------------------------------------------------------------------

// Manager loads and persists sessions as JSONL files.
type Manager struct {
	sessionsDir       string // workspace/sessions/
	legacySessionsDir string // ~/.nanobot/sessions/ (migration only)
	cache             sync.Map // key → *Session
}

// NewManager creates a Manager rooted at the workspace directory.
// It creates the sessions subdirectory if necessary.
func NewManager(workspace string) (*Manager, error) {
	dir := filepath.Join(workspace, "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create sessions dir: %w", err)
	}

	home, _ := os.UserHomeDir()
	legacy := filepath.Join(home, ".nanobot", "sessions")

	return &Manager{
		sessionsDir:       dir,
		legacySessionsDir: legacy,
	}, nil
}

// GetOrCreate returns the cached session for key, loading from disk if needed,
// or creating an empty new one.
func (m *Manager) GetOrCreate(key string) *Session {
	if v, ok := m.cache.Load(key); ok {
		return v.(*Session)
	}

	s := m.load(key)
	if s == nil {
		s = &Session{
			Key:       key,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Metadata:  map[string]any{},
		}
	}

	// Store under the canonical key; a concurrent goroutine may have beaten us —
	// prefer theirs to avoid a duplicate.
	actual, _ := m.cache.LoadOrStore(key, s)
	return actual.(*Session)
}

// Save writes the session to disk and updates the cache.
func (m *Manager) Save(s *Session) error {
	path := m.sessionPath(s.Key)

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // preserve non-ASCII, match Python ensure_ascii=False

	meta := map[string]any{
		"_type":             "metadata",
		"key":               s.Key,
		"created_at":        s.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":        time.Now().UTC().Format(time.RFC3339),
		"metadata":          s.Metadata,
		"last_consolidated": s.LastConsolidated,
	}
	if err := enc.Encode(meta); err != nil {
		return fmt.Errorf("encode metadata: %w", err)
	}

	s.mu.Lock()
	msgs := make([]map[string]any, len(s.Messages))
	copy(msgs, s.Messages)
	s.mu.Unlock()

	for _, msg := range msgs {
		if err := enc.Encode(msg); err != nil {
			return fmt.Errorf("encode message: %w", err)
		}
	}

	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write session %s: %w", path, err)
	}

	m.cache.Store(s.Key, s)
	return nil
}

// Invalidate removes a session from the in-memory cache (used after /new).
func (m *Manager) Invalidate(key string) {
	m.cache.Delete(key)
}

// ListSessions returns metadata for all sessions, sorted newest-first.
func (m *Manager) ListSessions() []map[string]any {
	entries, _ := filepath.Glob(filepath.Join(m.sessionsDir, "*.jsonl"))
	var out []map[string]any

	for _, path := range entries {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		if scanner.Scan() {
			var data map[string]any
			if json.Unmarshal(scanner.Bytes(), &data) == nil &&
				data["_type"] == "metadata" {
				key, _ := data["key"].(string)
				if key == "" {
					// Fall back: derive from filename
					base := filepath.Base(path)
					key = strings.TrimSuffix(base, ".jsonl")
					key = strings.Replace(key, "_", ":", 1)
				}
				out = append(out, map[string]any{
					"key":        key,
					"created_at": data["created_at"],
					"updated_at": data["updated_at"],
					"path":       path,
				})
			}
		}
		f.Close()
	}

	// Sort newest-first by updated_at string (ISO format sorts lexicographically).
	for i := 0; i < len(out)-1; i++ {
		for j := i + 1; j < len(out); j++ {
			ai, _ := out[i]["updated_at"].(string)
			aj, _ := out[j]["updated_at"].(string)
			if aj > ai {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Internal helpers

// sessionPath converts a session key to its JSONL file path.
// Mirrors Python: safe_filename(key.replace(":", "_")) + ".jsonl"
func (m *Manager) sessionPath(key string) string {
	name := safeFilename(strings.ReplaceAll(key, ":", "_"))
	return filepath.Join(m.sessionsDir, name+".jsonl")
}

// safeFilename replaces filesystem-unsafe characters with underscores.
// Matches Python's safe_filename: replaces <>:"/\|?* and trims whitespace.
func safeFilename(name string) string {
	const unsafe = `<>:"/\|?*`
	var b strings.Builder
	for _, r := range name {
		if strings.ContainsRune(unsafe, r) {
			b.WriteByte('_')
		} else {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// load reads a session from disk, migrating from the legacy path if needed.
func (m *Manager) load(key string) *Session {
	path := m.sessionPath(key)

	// Migrate from ~/.nanobot/sessions/ if the session exists there.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		legacyPath := filepath.Join(m.legacySessionsDir,
			safeFilename(strings.ReplaceAll(key, ":", "_"))+".jsonl")
		if _, err2 := os.Stat(legacyPath); err2 == nil {
			if err3 := os.Rename(legacyPath, path); err3 == nil {
				slog.Info("migrated session from legacy path", "key", key)
			}
		}
	}

	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var (
		messages         []map[string]any
		meta             = map[string]any{}
		createdAt        time.Time
		lastConsolidated int
	)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20) // 1 MB per line
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		var data map[string]any
		if err := json.Unmarshal(line, &data); err != nil {
			slog.Warn("skipping malformed session line", "key", key, "err", err)
			continue
		}

		if data["_type"] == "metadata" {
			if m2, ok := data["metadata"].(map[string]any); ok {
				meta = m2
			}
			if ts, ok := data["created_at"].(string); ok {
				if t, err := time.Parse(time.RFC3339, ts); err == nil {
					createdAt = t
				}
			}
			if lc, ok := data["last_consolidated"].(float64); ok {
				lastConsolidated = int(lc)
			}
		} else {
			messages = append(messages, data)
		}
	}

	if err := scanner.Err(); err != nil {
		slog.Warn("error reading session file", "key", key, "err", err)
		return nil
	}

	if createdAt.IsZero() {
		createdAt = time.Now()
	}

	return &Session{
		Key:              key,
		Messages:         messages,
		CreatedAt:        createdAt,
		UpdatedAt:        time.Now(),
		Metadata:         meta,
		LastConsolidated: lastConsolidated,
	}
}
