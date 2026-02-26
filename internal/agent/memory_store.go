// Package agent contains the core agent loop and its supporting components.
package agent

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/crystaldolphin/crystaldolphin/internal/schema"
)

type FileMemoryStore struct {
	memoryDir       string
	memoryFilePath  string
	historyFilePath string
}

// NewMemoryStore creates a FileMemoryStore rooted at workspace.
// The memory/ subdirectory is created if it does not exist.
func NewMemoryStore(workspace string) (schema.MemoryStore, error) {
	dir := filepath.Join(workspace, "memory")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create memory dir: %w", err)
	}

	return &FileMemoryStore{
		memoryDir:       dir,
		memoryFilePath:  filepath.Join(dir, "MEMORY.md"),
		historyFilePath: filepath.Join(dir, "HISTORY.md"),
	}, nil
}

// ReadLongTerm returns the current contents of MEMORY.md, or "" if not yet written.
func (m *FileMemoryStore) ReadLongTerm() string {
	data, err := os.ReadFile(m.memoryFilePath)
	if err != nil {
		return ""
	}
	return string(data)
}

// WriteLongTerm overwrites MEMORY.md with content.
func (m *FileMemoryStore) WriteLongTerm(content string) error {
	return os.WriteFile(m.memoryFilePath, []byte(content), 0o644)
}

// AppendHistory appends a timestamped entry to HISTORY.md followed by a blank line.
func (m *FileMemoryStore) AppendHistory(entry string) error {
	f, err := os.OpenFile(m.historyFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open history file: %w", err)
	}
	defer f.Close()

	// Strip trailing whitespace, add double newline (matches Python behaviour).
	line := entry
	for len(line) > 0 && (line[len(line)-1] == '\n' || line[len(line)-1] == '\r' || line[len(line)-1] == ' ') {
		line = line[:len(line)-1]
	}
	_, err = fmt.Fprintf(f, "%s\n\n", line)
	return err
}

// GetMemoryContext returns the long-term memory formatted for injection into
// the system prompt, or "" if MEMORY.md is empty.
func (m *FileMemoryStore) GetMemoryContext() string {
	longTerm := m.ReadLongTerm()
	if longTerm == "" {
		return ""
	}

	return "## Long-term Memory\n" + longTerm
}
