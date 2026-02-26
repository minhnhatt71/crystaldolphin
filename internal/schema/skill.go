package schema

// SkillInfo holds metadata about a single skill.
type SkillInfo struct {
	Name   string // directory name
	Path   string // absolute path to SKILL.md
	Source string // "workspace" or "builtin"
}

// SkillLoader is the interface for loading and querying agent skills.
// SkillsLoader in the agent package is the canonical implementation.
type SkillLoader interface {
	ListSkills(filterUnavailable bool) []SkillInfo
	LoadSkill(name string) string
	LoadSkillsForContext(names []string) string
	BuildSkillsSummary() string
	GetAlwaysSkills() []string
}
