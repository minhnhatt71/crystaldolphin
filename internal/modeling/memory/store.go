package memory

type Store interface {
	ReadLongterm() (LongTerm, error)
	WriteLongterm(memory LongTerm) error

	ReadHistory() (History, error)
	WriteHistory(history History) error
}
