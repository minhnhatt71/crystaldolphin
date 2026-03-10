package session

import "time"

// Role identifies the speaker of a session entry.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
	RoleSystem    Role = "system"
)

// SessionEntry is the value type for a single stored message row in a session.
// It is a pure value object with no I/O.
type SessionEntry struct {
	role       Role
	content    string
	toolsUsed  []string
	toolCallId string
	timestamp  time.Time
}

// --- Accessors ---

func (e SessionEntry) Role() Role           { return e.role }
func (e SessionEntry) Content() string      { return e.content }
func (e SessionEntry) ToolsUsed() []string  { return e.toolsUsed }
func (e SessionEntry) ToolCallId() string   { return e.toolCallId }
func (e SessionEntry) Timestamp() time.Time { return e.timestamp }

// --- Factories ---

func NewUserEntry(content string) SessionEntry {
	return SessionEntry{role: RoleUser, content: content, timestamp: time.Now()}
}

func NewAssistantEntry(content string, toolsUsed []string) SessionEntry {
	return SessionEntry{role: RoleAssistant, content: content, toolsUsed: toolsUsed, timestamp: time.Now()}
}

func NewToolEntry(toolCallID, content string) SessionEntry {
	return SessionEntry{role: RoleTool, toolCallId: toolCallID, content: content, timestamp: time.Now()}
}

func NewSystemEntry(content string) SessionEntry {
	return SessionEntry{role: RoleSystem, content: content, timestamp: time.Now()}
}
