package skills

// Loader provides access to skill definitions stored in the workspace.
// Concrete implementations live outside modeling/.
type Loader interface {
	// GetAlwaysSkills returns the names of skills that should always be
	// included in full in the system prompt.
	GetAlwaysSkills() []string

	// LoadSkillsForContext returns the combined content of the named skills,
	// ready for injection into the system prompt.
	LoadSkillsForContext(names []string) string

	// BuildSkillsSummary returns a short directory listing of all available
	// skills for progressive loading.
	BuildSkillsSummary() string
}
