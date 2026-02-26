// Package session manages per-conversation history stored as JSONL files.
//
// File format (byte-compatible with nanobot Python):
//
//	Line 1:  {"_type":"metadata","key":"…","created_at":"…","updated_at":"…",
//	           "metadata":{…},"last_consolidated":N}
//	Line 2+: one JSON message object per line
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

	"github.com/crystaldolphin/crystaldolphin/internal/schema"
)

// Manager loads and persists sessions as JSONL files.
type Manager struct {
	sessionsDir string   // workspace/sessions/
	cache       sync.Map // key → *Session
}

// NewManager creates a Manager rooted at the workspace directory.
// It creates the sessions subdirectory if necessary.
func NewManager(workspace string) (*Manager, error) {
	dir := filepath.Join(workspace, "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create sessions dir: %w", err)
	}

	return &Manager{sessionsDir: dir}, nil
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
			Messages:  schema.NewMessages(),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Metadata:  map[string]any{},
		}
	}

	actual, _ := m.cache.LoadOrStore(key, s)

	return actual.(*Session)
}

// Save writes the session to disk and updates the cache.
func (m *Manager) Save(s *Session) error {
	path := m.sessionPath(s.Key)

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // preserve non-ASCII, match Python ensure_ascii=False

	s.mu.Lock()
	msgs := s.Messages.Clone()
	meta := map[string]any{
		"_type":             "metadata",
		"key":               s.Key,
		"created_at":        s.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":        time.Now().UTC().Format(time.RFC3339),
		"metadata":          s.Metadata,
		"last_consolidated": s.LastConsolidated,
	}
	s.mu.Unlock()

	if err := enc.Encode(meta); err != nil {
		return fmt.Errorf("encode metadata: %w", err)
	}

	for _, msg := range msgs.Messages {
		wire := messageToWire(msg)
		if err := enc.Encode(wire); err != nil {
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
// Wire format helpers

// wireMessage is the on-disk JSON representation of a message.
// It mirrors the nanobot Python format exactly.
type wireMessage struct {
	Role             string           `json:"role"`
	Content          any              `json:"content"`
	ToolCalls        []map[string]any `json:"tool_calls,omitempty"`
	ToolCallID       string           `json:"tool_call_id,omitempty"`
	Name             string           `json:"name,omitempty"`
	ReasoningContent string           `json:"reasoning_content,omitempty"`
	ToolsUsed        []string         `json:"tools_used,omitempty"`
	Timestamp        string           `json:"timestamp"`
}

// messageToWire converts a typed Message to its on-disk map representation.
func messageToWire(msg schema.Message) wireMessage {
	w := wireMessage{
		Role:      msg.Role,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		ToolsUsed: msg.ToolsUsed,
	}

	switch v := msg.Content.(type) {
	case string:
		w.Content = v
	case *string:
		if v != nil {
			w.Content = *v
		}
	default:
		w.Content = msg.Content
	}

	if msg.ReasoningContent != nil {
		w.ReasoningContent = *msg.ReasoningContent
	}

	for _, tc := range msg.ToolCalls {
		w.ToolCalls = append(w.ToolCalls, tc.ToWireMap())
	}

	w.ToolCallID = msg.ToolCallID
	w.Name = msg.ToolName

	return w
}

// wireToMessage converts an on-disk wire map back to a typed Message.
func wireToMessage(data map[string]any) schema.Message {
	role, _ := data["role"].(string)
	content := data["content"]
	if content == nil {
		content = ""
	}

	msg := schema.Message{
		Role:    role,
		Content: content,
	}

	// Restore tool calls stored in session as []any of wire-format maps.
	if tcs, ok := data["tool_calls"].([]any); ok {
		for _, tc := range tcs {
			tcm, ok := tc.(map[string]any)
			if !ok {
				continue
			}
			fn, _ := tcm["function"].(map[string]any)
			id, _ := tcm["id"].(string)
			name, _ := fn["name"].(string)
			argsStr, _ := fn["arguments"].(string)
			var args map[string]any
			_ = json.Unmarshal([]byte(argsStr), &args)
			msg.ToolCalls = append(msg.ToolCalls, schema.ToolCall{
				ID:        id,
				Name:      name,
				Arguments: args,
			})
		}
	}

	if id, ok := data["tool_call_id"].(string); ok {
		msg.ToolCallID = id
	}
	if name, ok := data["name"].(string); ok {
		msg.ToolName = name
	}
	if rc, ok := data["reasoning_content"].(string); ok && rc != "" {
		msg.ReasoningContent = &rc
	}
	if tu, ok := data["tools_used"].([]any); ok {
		for _, t := range tu {
			if s, ok := t.(string); ok {
				msg.ToolsUsed = append(msg.ToolsUsed, s)
			}
		}
	}

	return msg
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

	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var (
		messages         schema.Messages
		meta             = map[string]any{}
		createdAt        time.Time
		lastConsolidated int
	)

	messages = schema.NewMessages()

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
			messages.Add(wireToMessage(data))
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
