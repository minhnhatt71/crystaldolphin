package prompt

type Prompts struct {
	prompts []Prompt
}

func NewList(prompts ...Prompt) Prompts {
	return Prompts{
		prompts: prompts,
	}
}

func (p Prompts) Add(prompt Prompt) Prompts {
	p.prompts = append(p.prompts, prompt)

	return p
}

func (p Prompts) Get() []Prompt {
	return p.prompts
}
