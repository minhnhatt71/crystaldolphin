package memory

// LongTerm is the value type representing the agent's long-term memory
// (the content of MEMORY.md). It is a pure value object with no I/O.
type LongTerm struct {
	content string
}

func NewLongTerm(content string) LongTerm {
	return LongTerm{content: content}
}

func (m LongTerm) Content() string {
	return m.content
}

func (m LongTerm) IsEmpty() bool {
	return m.content == ""
}
