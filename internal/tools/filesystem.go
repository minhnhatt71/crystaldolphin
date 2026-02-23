package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// resolvePath resolves a file path against workspace (if relative) and enforces
// directory restriction if allowedDir is non-empty.
// Mirrors Python's _resolve_path().
func resolvePath(path, workspace, allowedDir string) (string, error) {
	p := path
	if !filepath.IsAbs(p) && workspace != "" {
		p = filepath.Join(workspace, p)
	}
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		// Path may not exist yet (for writes) — use Clean instead
		resolved = filepath.Clean(p)
	}
	if allowedDir != "" {
		allowedResolved := filepath.Clean(allowedDir)
		if !strings.HasPrefix(resolved, allowedResolved) {
			return "", fmt.Errorf("path %s is outside allowed directory %s", path, allowedDir)
		}
	}
	return resolved, nil
}

// ---------------------------------------------------------------------------
// ReadFileTool
// ---------------------------------------------------------------------------

// ReadFileTool reads a file and returns its contents.
type ReadFileTool struct {
	workspace  string
	allowedDir string
}

func NewReadFileTool(workspace, allowedDir string) *ReadFileTool {
	return &ReadFileTool{workspace: workspace, allowedDir: allowedDir}
}

func (t *ReadFileTool) Name() string        { return "read_file" }
func (t *ReadFileTool) Description() string { return "Read the contents of a file at the given path." }
func (t *ReadFileTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "The file path to read"
			}
		},
		"required": ["path"]
	}`)
}

func (t *ReadFileTool) Execute(_ context.Context, params map[string]any) (string, error) {
	path, _ := params["path"].(string)
	if path == "" {
		return "Error: path is required", nil
	}
	fp, err := resolvePath(path, t.workspace, t.allowedDir)
	if err != nil {
		return "Error: " + err.Error(), nil
	}
	info, err := os.Stat(fp)
	if err != nil {
		return fmt.Sprintf("Error: File not found: %s", path), nil
	}
	if !info.Mode().IsRegular() {
		return fmt.Sprintf("Error: Not a file: %s", path), nil
	}
	data, err := os.ReadFile(fp)
	if err != nil {
		return fmt.Sprintf("Error reading file: %s", err), nil
	}
	return string(data), nil
}

// ---------------------------------------------------------------------------
// WriteFileTool
// ---------------------------------------------------------------------------

// WriteFileTool writes content to a file, creating parent directories as needed.
type WriteFileTool struct {
	workspace  string
	allowedDir string
}

func NewWriteFileTool(workspace, allowedDir string) *WriteFileTool {
	return &WriteFileTool{workspace: workspace, allowedDir: allowedDir}
}

func (t *WriteFileTool) Name() string { return "write_file" }
func (t *WriteFileTool) Description() string {
	return "Write content to a file at the given path. Creates parent directories if needed."
}
func (t *WriteFileTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "The file path to write to"
			},
			"content": {
				"type": "string",
				"description": "The content to write"
			}
		},
		"required": ["path", "content"]
	}`)
}

func (t *WriteFileTool) Execute(_ context.Context, params map[string]any) (string, error) {
	path, _ := params["path"].(string)
	content, _ := params["content"].(string)
	if path == "" {
		return "Error: path is required", nil
	}
	fp, err := resolvePath(path, t.workspace, t.allowedDir)
	if err != nil {
		return "Error: " + err.Error(), nil
	}
	if err := os.MkdirAll(filepath.Dir(fp), 0o755); err != nil {
		return fmt.Sprintf("Error creating directories: %s", err), nil
	}
	if err := os.WriteFile(fp, []byte(content), 0o644); err != nil {
		return fmt.Sprintf("Error writing file: %s", err), nil
	}
	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), fp), nil
}

// ---------------------------------------------------------------------------
// EditFileTool
// ---------------------------------------------------------------------------

// EditFileTool replaces old_text with new_text in a file (first occurrence).
type EditFileTool struct {
	workspace  string
	allowedDir string
}

func NewEditFileTool(workspace, allowedDir string) *EditFileTool {
	return &EditFileTool{workspace: workspace, allowedDir: allowedDir}
}

func (t *EditFileTool) Name() string { return "edit_file" }
func (t *EditFileTool) Description() string {
	return "Edit a file by replacing old_text with new_text. The old_text must exist exactly in the file."
}
func (t *EditFileTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "The file path to edit"
			},
			"old_text": {
				"type": "string",
				"description": "The exact text to find and replace"
			},
			"new_text": {
				"type": "string",
				"description": "The text to replace with"
			}
		},
		"required": ["path", "old_text", "new_text"]
	}`)
}

func (t *EditFileTool) Execute(_ context.Context, params map[string]any) (string, error) {
	path, _ := params["path"].(string)
	oldText, _ := params["old_text"].(string)
	newText, _ := params["new_text"].(string)
	if path == "" {
		return "Error: path is required", nil
	}

	fp, err := resolvePath(path, t.workspace, t.allowedDir)
	if err != nil {
		return "Error: " + err.Error(), nil
	}
	data, err := os.ReadFile(fp)
	if err != nil {
		return fmt.Sprintf("Error: File not found: %s", path), nil
	}
	content := string(data)

	if !strings.Contains(content, oldText) {
		return editNotFoundMessage(oldText, content, path), nil
	}
	count := strings.Count(content, oldText)
	if count > 1 {
		return fmt.Sprintf("Warning: old_text appears %d times. Please provide more context to make it unique.", count), nil
	}

	newContent := strings.Replace(content, oldText, newText, 1)
	if err := os.WriteFile(fp, []byte(newContent), 0o644); err != nil {
		return fmt.Sprintf("Error writing file: %s", err), nil
	}
	return fmt.Sprintf("Successfully edited %s", fp), nil
}

// editNotFoundMessage builds a helpful diff hint when old_text is not found.
// Mirrors Python's EditFileTool._not_found_message() using a sliding window.
func editNotFoundMessage(oldText, content, path string) string {
	oldLines := strings.Split(oldText, "\n")
	contentLines := strings.Split(content, "\n")
	window := len(oldLines)

	bestRatio := 0.0
	bestStart := 0

	end := len(contentLines) - window + 1
	if end < 1 {
		end = 1
	}
	for i := 0; i < end; i++ {
		r := similarityRatio(oldLines, contentLines[i:i+window])
		if r > bestRatio {
			bestRatio, bestStart = r, i
		}
	}

	if bestRatio > 0.5 {
		return fmt.Sprintf(
			"Error: old_text not found in %s.\nBest match (%.0f%% similar) at line %d:\n%s",
			path, bestRatio*100, bestStart+1,
			unifiedDiffHint(oldLines, contentLines[bestStart:bestStart+window], path, bestStart),
		)
	}
	return fmt.Sprintf("Error: old_text not found in %s. No similar text found. Verify the file content.", path)
}

// similarityRatio computes a simple character-level overlap ratio.
func similarityRatio(a, b []string) float64 {
	sa := strings.Join(a, "\n")
	sb := strings.Join(b, "\n")
	if len(sa)+len(sb) == 0 {
		return 1.0
	}
	common := 0
	// count common bytes (order-independent approximation)
	freq := make(map[byte]int)
	for i := 0; i < len(sa); i++ {
		freq[sa[i]]++
	}
	for i := 0; i < len(sb); i++ {
		if freq[sb[i]] > 0 {
			common++
			freq[sb[i]]--
		}
	}
	return 2.0 * float64(common) / float64(len(sa)+len(sb))
}

// unifiedDiffHint returns a simple unified-diff-like hint.
func unifiedDiffHint(oldLines, newLines []string, path string, startLine int) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("--- old_text (provided)\n+++ %s (actual, line %d)\n", path, startLine+1))
	max := len(oldLines)
	if len(newLines) > max {
		max = len(newLines)
	}
	for i := 0; i < max; i++ {
		if i < len(oldLines) {
			sb.WriteString("- " + oldLines[i] + "\n")
		}
		if i < len(newLines) {
			sb.WriteString("+ " + newLines[i] + "\n")
		}
	}
	return sb.String()
}

// ---------------------------------------------------------------------------
// ListDirTool
// ---------------------------------------------------------------------------

// ListDirTool lists directory contents.
type ListDirTool struct {
	workspace  string
	allowedDir string
}

func NewListDirTool(workspace, allowedDir string) *ListDirTool {
	return &ListDirTool{workspace: workspace, allowedDir: allowedDir}
}

func (t *ListDirTool) Name() string        { return "list_dir" }
func (t *ListDirTool) Description() string { return "List the contents of a directory." }
func (t *ListDirTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "The directory path to list"
			}
		},
		"required": ["path"]
	}`)
}

func (t *ListDirTool) Execute(_ context.Context, params map[string]any) (string, error) {
	path, _ := params["path"].(string)
	if path == "" {
		return "Error: path is required", nil
	}
	dp, err := resolvePath(path, t.workspace, t.allowedDir)
	if err != nil {
		return "Error: " + err.Error(), nil
	}
	info, err := os.Stat(dp)
	if err != nil {
		return fmt.Sprintf("Error: Directory not found: %s", path), nil
	}
	if !info.IsDir() {
		return fmt.Sprintf("Error: Not a directory: %s", path), nil
	}
	entries, err := os.ReadDir(dp)
	if err != nil {
		return fmt.Sprintf("Error listing directory: %s", err), nil
	}
	if len(entries) == 0 {
		return fmt.Sprintf("Directory %s is empty", path), nil
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	var lines []string
	for _, e := range entries {
		prefix := "[F] "
		if e.IsDir() {
			prefix = "[D] "
		}
		lines = append(lines, prefix+e.Name())
	}
	return strings.Join(lines, "\n"), nil
}
