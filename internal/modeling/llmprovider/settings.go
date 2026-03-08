package llmprovider

type Settings struct {
	model         string
	maxIterations int
	temperature   float64
	maxTokens     int
	memoryWindow  int
}

func NewSettings(model string, maxIteration int, temperature float64, maxTokens int, memoryWindow int) Settings {
	return Settings{
		model:         model,
		maxIterations: maxIteration,
		temperature:   temperature,
		maxTokens:     maxTokens,
		memoryWindow:  memoryWindow,
	}
}

func (s Settings) Model() string        { return s.model }
func (s Settings) MaxIterations() int   { return s.maxIterations }
func (s Settings) Temperature() float64 { return s.temperature }
func (s Settings) MaxTokens() int       { return s.maxTokens }
func (s Settings) MemoryWindow() int    { return s.memoryWindow }
