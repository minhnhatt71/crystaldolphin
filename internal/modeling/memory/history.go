package memory

import "time"

// History holds an ordered log of summarised conversation entries
// (the content of HISTORY.md). It is a pure value object with no I/O.
type History struct {
	entries []HistoryEntry
}

func NewHistory() History {
	return History{}
}

func (h *History) Append(entry HistoryEntry) {
	h.entries = append(h.entries, entry)
}

func (h History) Entries() []HistoryEntry {
	return h.entries
}

// HistoryEntry is a single summarised turn stored in HISTORY.md.
type HistoryEntry struct {
	content   string
	timestamp time.Time
}

func NewHistoryEntry(content string, ts time.Time) HistoryEntry {
	return HistoryEntry{content: content, timestamp: ts}
}

func (e HistoryEntry) Content() string {
	return e.content
}

func (e HistoryEntry) Timestamp() time.Time {
	return e.timestamp
}
