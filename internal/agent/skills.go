package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/crystaldolphin/crystaldolphin/internal/schema"
)

// skillMeta is the YAML frontmatter structure of a SKILL.md file.
type skillMeta struct {
	Description string `yaml:"description"`
	Always      bool   `yaml:"always"`
	// Nested JSON string under "metadata" key
	Metadata string `yaml:"metadata"`
}

// crystalDolphinMeta is the structure inside the JSON "metadata" YAML field.
type crystalDolphinMeta struct {
	Always   bool `json:"always"`
	Requires struct {
		Bins []string `json:"bins"`
		Env  []string `json:"env"`
	} `json:"requires"`
}

// SkillsLoader scans workspace and builtin skills directories and builds
type SkillsLoader struct {
	workspace       string // workspace root (contains skills/ subdir)
	workspaceSkills string
	builtinSkills   string // path to embedded/bundled skills root
}

// NewSkillsLoader creates a SkillsLoader.
// builtinSkillsDir may be "" if there is no embedded skills directory.
func NewSkillsLoader(workspace, builtinSkillsDir string) *SkillsLoader {
	return &SkillsLoader{
		workspace:       workspace,
		workspaceSkills: filepath.Join(workspace, "skills"),
		builtinSkills:   builtinSkillsDir,
	}
}

// ListSkills returns all available skills.
// If filterUnavailable is true, skills with unmet requirements are excluded.
func (sl *SkillsLoader) ListSkills(filterUnavailable bool) []schema.SkillInfo {
	seen := map[string]bool{}
	var skills []schema.SkillInfo

	// Workspace skills have highest priority.
	if entries, err := os.ReadDir(sl.workspaceSkills); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			p := filepath.Join(sl.workspaceSkills, e.Name(), "SKILL.md")
			if _, err := os.Stat(p); err == nil {
				skills = append(skills, schema.SkillInfo{Name: e.Name(), Path: p, Source: "workspace"})
				seen[e.Name()] = true
			}
		}
	}

	// Builtin skills.
	if sl.builtinSkills != "" {
		if entries, err := os.ReadDir(sl.builtinSkills); err == nil {
			for _, e := range entries {
				if !e.IsDir() || seen[e.Name()] {
					continue
				}
				p := filepath.Join(sl.builtinSkills, e.Name(), "SKILL.md")
				if _, err := os.Stat(p); err == nil {
					skills = append(skills, schema.SkillInfo{Name: e.Name(), Path: p, Source: "builtin"})
				}
			}
		}
	}

	if !filterUnavailable {
		return skills
	}
	var out []schema.SkillInfo
	for _, s := range skills {
		m := sl.getCrystalDolphinMeta(s.Name)
		if sl.checkRequirements(m) {
			out = append(out, s)
		}
	}
	return out
}

// LoadSkill returns the raw content of a skill's SKILL.md, or "".
func (sl *SkillsLoader) LoadSkill(name string) string {
	// Workspace first.
	p := filepath.Join(sl.workspaceSkills, name, "SKILL.md")
	if data, err := os.ReadFile(p); err == nil {
		return string(data)
	}
	if sl.builtinSkills != "" {
		p = filepath.Join(sl.builtinSkills, name, "SKILL.md")
		if data, err := os.ReadFile(p); err == nil {
			return string(data)
		}
	}
	return ""
}

// LoadSkillsForContext loads a set of named skills and returns them formatted
// for inclusion in the system prompt (frontmatter stripped).
func (sl *SkillsLoader) LoadSkillsForContext(names []string) string {
	var parts []string
	for _, name := range names {
		content := sl.LoadSkill(name)
		if content == "" {
			continue
		}
		content = stripFrontmatter(content)
		parts = append(parts, fmt.Sprintf("### Skill: %s\n\n%s", name, content))
	}
	return strings.Join(parts, "\n\n---\n\n")
}

// BuildSkillsSummary returns an XML summary of all skills for progressive loading.
func (sl *SkillsLoader) BuildSkillsSummary() string {
	all := sl.ListSkills(false)
	if len(all) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("<skills>\n")
	for _, s := range all {
		m := sl.getCrystalDolphinMeta(s.Name)
		available := sl.checkRequirements(m)
		desc := sl.getSkillDescription(s.Name)

		fmt.Fprintf(&sb, "  <skill available=%q>\n", fmt.Sprintf("%v", available))
		fmt.Fprintf(&sb, "    <name>%s</name>\n", xmlEscape(s.Name))
		fmt.Fprintf(&sb, "    <description>%s</description>\n", xmlEscape(desc))
		fmt.Fprintf(&sb, "    <location>%s</location>\n", s.Path)
		if !available {
			missing := sl.getMissingRequirements(m)
			if missing != "" {
				fmt.Fprintf(&sb, "    <requires>%s</requires>\n", xmlEscape(missing))
			}
		}
		sb.WriteString("  </skill>\n")
	}
	sb.WriteString("</skills>")
	return sb.String()
}

// GetAlwaysSkills returns names of skills marked always=true with met requirements.
func (sl *SkillsLoader) GetAlwaysSkills() []string {
	var result []string
	for _, s := range sl.ListSkills(true) {
		fm := sl.getSkillFrontmatter(s.Name)
		nm := sl.getCrystalDolphinMeta(s.Name)
		if fm.Always || nm.Always {
			result = append(result, s.Name)
		}
	}

	return result
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func (sl *SkillsLoader) getSkillFrontmatter(name string) skillMeta {
	content := sl.LoadSkill(name)
	if content == "" || !strings.HasPrefix(content, "---") {
		return skillMeta{}
	}
	// Extract YAML block between first --- and second ---
	rest := content[3:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return skillMeta{}
	}
	yamlBlock := rest[:end]
	var m skillMeta
	_ = yaml.Unmarshal([]byte(yamlBlock), &m)
	return m
}

func (sl *SkillsLoader) getCrystalDolphinMeta(name string) crystalDolphinMeta {
	fm := sl.getSkillFrontmatter(name)
	if fm.Metadata == "" {
		return crystalDolphinMeta{}
	}
	var nm crystalDolphinMeta
	// Metadata field can be a JSON string.
	raw := fm.Metadata
	// Try wrapping with {"nanobot": ...} if it looks like a JSON object.
	var outer map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &outer); err == nil {
		if nb, ok := outer["nanobot"]; ok {
			_ = json.Unmarshal(nb, &nm)
			return nm
		}
		if oc, ok := outer["openclaw"]; ok {
			_ = json.Unmarshal(oc, &nm)
			return nm
		}
		// Raw JSON is already the nanobot meta.
		_ = json.Unmarshal([]byte(raw), &nm)
	}
	return nm
}

func (sl *SkillsLoader) getSkillDescription(name string) string {
	fm := sl.getSkillFrontmatter(name)
	if fm.Description != "" {
		return fm.Description
	}
	return name
}

func (sl *SkillsLoader) checkRequirements(m crystalDolphinMeta) bool {
	for _, bin := range m.Requires.Bins {
		if _, err := exec.LookPath(bin); err != nil {
			return false
		}
	}
	for _, env := range m.Requires.Env {
		if os.Getenv(env) == "" {
			return false
		}
	}
	return true
}

func (sl *SkillsLoader) getMissingRequirements(m crystalDolphinMeta) string {
	var missing []string
	for _, bin := range m.Requires.Bins {
		if _, err := exec.LookPath(bin); err != nil {
			missing = append(missing, "CLI: "+bin)
		}
	}
	for _, env := range m.Requires.Env {
		if os.Getenv(env) == "" {
			missing = append(missing, "ENV: "+env)
		}
	}
	return strings.Join(missing, ", ")
}

// stripFrontmatter removes the leading --- ... --- YAML block from markdown.
func stripFrontmatter(content string) string {
	if !strings.HasPrefix(content, "---") {
		return content
	}
	rest := content[3:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return content
	}
	return strings.TrimSpace(rest[end+4:])
}

// xmlEscape escapes &, <, > for XML attribute/text use.
func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
