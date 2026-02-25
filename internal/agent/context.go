package agent

import (
	"encoding/base64"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ContextBuilder assembles system prompts and message lists for the LLM.
// Mirrors nanobot's Python ContextBuilder.
type ContextBuilder struct {
	workspace string
	memory    *MemoryStore
	skills    *SkillsLoader
}

// bootstrapFiles lists workspace files loaded into the system prompt.
var bootstrapFiles = []string{"AGENTS.md", "SOUL.md", "USER.md", "TOOLS.md", "IDENTITY.md"}

// NewContextBuilder creates a ContextBuilder for the given workspace.
// builtinSkillsDir may be "" if there is no embedded skills directory.
// Memory directory creation errors are silently ignored.
func NewContextBuilder(workspace, builtinSkillsDir string) *ContextBuilder {
	mem, _ := NewMemoryStore(workspace)
	if mem == nil {
		mem = &MemoryStore{}
	}
	return &ContextBuilder{
		workspace: workspace,
		memory:    mem,
		skills:    NewSkillsLoader(workspace, builtinSkillsDir),
	}
}

// BuildSystemPrompt assembles the full system prompt: identity + bootstrap
// files + memory + always-skills + skills summary.
func (cb *ContextBuilder) BuildSystemPrompt() string {
	var parts []string

	parts = append(parts, cb.buildIdentity())

	if bootstrap := cb.loadBootstrapFiles(); bootstrap != "" {
		parts = append(parts, bootstrap)
	}

	if mem := cb.memory.GetMemoryContext(); mem != "" {
		parts = append(parts, "# Memory\n\n"+mem)
	}

	// Always-loaded skills (full content).
	if alwaysNames := cb.skills.GetAlwaysSkills(); len(alwaysNames) > 0 {
		if content := cb.skills.LoadSkillsForContext(alwaysNames); content != "" {
			parts = append(parts, "# Active Skills\n\n"+content)
		}
	}

	// Skills directory summary (progressive loading).
	if summary := cb.skills.BuildSkillsSummary(); summary != "" {
		parts = append(parts, `# Skills

The following skills extend your capabilities. To use a skill, read its SKILL.md file using the read_file tool.
Skills with available="false" need dependencies installed first - you can try installing them with apt/brew.

`+summary)
	}

	return strings.Join(parts, "\n\n---\n\n")
}

// buildIdentity returns the core identity section of the system prompt.
func (cb *ContextBuilder) buildIdentity() string {
	now := time.Now().Format("2006-01-02 15:04 (Monday)")
	tz, _ := time.Now().Zone()
	if tz == "" {
		tz = "UTC"
	}
	wsExpanded := expandHome(cb.workspace)
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	osName := goos
	if goos == "darwin" {
		osName = "macOS"
	}
	runtimeStr := fmt.Sprintf("%s %s, Go %s", osName, goarch, runtime.Version())

	return fmt.Sprintf(`# crystaldolphin 🐈

You are crystaldolphin, a helpful AI assistant.

## Current Time
%s (%s)

## Runtime
%s

## Workspace
Your workspace is at: %s
- Long-term memory: %s/memory/MEMORY.md
- History log: %s/memory/HISTORY.md (grep-searchable)
- Custom skills: %s/skills/{skill-name}/SKILL.md

IMPORTANT: When responding to direct questions or conversations, reply directly with your text response.
Only use the 'message' tool when you need to send a message to a specific chat channel (like WhatsApp).
For normal conversation, just respond with text - do not call the message tool.

Always be helpful, accurate, and concise. Before calling tools, briefly tell the user what you're about to do (one short sentence in the user's language).
If you need to use tools, call them directly — never send a preliminary message like "Let me check" without actually calling a tool.
When remembering something important, write to %s/memory/MEMORY.md
To recall past events, grep %s/memory/HISTORY.md`,
		now, tz,
		runtimeStr,
		wsExpanded,
		wsExpanded, wsExpanded, wsExpanded,
		wsExpanded, wsExpanded,
	)
}

// loadBootstrapFiles reads all bootstrap markdown files from the workspace.
func (cb *ContextBuilder) loadBootstrapFiles() string {
	var parts []string
	for _, name := range bootstrapFiles {
		p := filepath.Join(cb.workspace, name)
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		parts = append(parts, fmt.Sprintf("## %s\n\n%s", name, string(data)))
	}
	return strings.Join(parts, "\n\n")
}

// BuildMessages builds the complete message list for an LLM call.
// Mirrors Python ContextBuilder.build_messages().
func (cb *ContextBuilder) BuildMessages(
	history MessageHistory,
	currentMessage string,
	media []string,
	channel, chatID string,
) MessageHistory {
	systemPrompt := cb.BuildSystemPrompt()
	if channel != "" && chatID != "" {
		systemPrompt += fmt.Sprintf("\n\n## Current Session\nChannel: %s\nChat ID: %s", channel, chatID)
	}

	messages := NewMessageHistory()
	messages.AddSystem(systemPrompt)
	messages.Append(history)
	messages.AddUser(cb.buildUserContent(currentMessage, media))

	return messages
}

// buildUserContent builds user content, embedding base64 images when media is provided.
func (cb *ContextBuilder) buildUserContent(text string, media []string) any {
	if len(media) == 0 {
		return text
	}

	var blocks []map[string]any
	for _, path := range media {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		mimeType := mime.TypeByExtension(filepath.Ext(path))
		if mimeType == "" || !strings.HasPrefix(mimeType, "image/") {
			continue
		}
		b64 := base64.StdEncoding.EncodeToString(data)
		blocks = append(blocks, map[string]any{
			"type":      "image_url",
			"image_url": map[string]any{"url": fmt.Sprintf("data:%s;base64,%s", mimeType, b64)},
		})
	}

	if len(blocks) == 0 {
		return text
	}
	return append(blocks, map[string]any{"type": "text", "text": text})
}

// expandHome replaces a leading "~" with the user's home directory.
func expandHome(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[1:])
}
