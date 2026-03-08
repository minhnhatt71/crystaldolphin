package prompt

type Prompt struct {
	role      string
	content   *string
	tools     []Tool
	reasoning *string
}

func (p *Prompt) Role() string              { return p.role }
func (p *Prompt) Content() *string          { return p.content }
func (p *Prompt) Tools() []Tool             { return p.tools }
func (p *Prompt) ReasoningContent() *string { return p.reasoning }

type Tool interface {
	Id() string
	Name() string
	Arguments() map[string]any
}

type Tools interface {
	List() []Tool
	Add(tool Tool)
	Get(name string) Tool
}
