package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// denyPatterns mirrors Python ExecTool's deny_patterns exactly.
var denyPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\brm\s+-[rf]{1,2}\b`),            // rm -r, rm -rf, rm -fr
	regexp.MustCompile(`(?i)\bdel\s+/[fq]\b`),                // del /f, del /q
	regexp.MustCompile(`(?i)\brmdir\s+/s\b`),                 // rmdir /s
	regexp.MustCompile(`(?i)(?:^|[;&|]\s*)format\b`),         // format (standalone)
	regexp.MustCompile(`(?i)\b(mkfs|diskpart)\b`),            // disk ops
	regexp.MustCompile(`(?i)\bdd\s+if=`),                     // dd
	regexp.MustCompile(`(?i)>\s*/dev/sd`),                    // write to disk
	regexp.MustCompile(`(?i)\b(shutdown|reboot|poweroff)\b`), // power control
	regexp.MustCompile(`:\(\)\s*\{.*\};\s*:`),                // fork bomb
}

// ExecTool executes shell commands with safety guards.
type ExecTool struct {
	timeout             time.Duration
	workingDir          string
	restrictToWorkspace bool
}

// NewExecTool creates an ExecTool.
// workingDir is the default CWD (empty = os.Getwd()).
// restrictToWorkspace enables workspace path restriction.
func NewExecTool(workingDir string, timeoutSeconds int, restrictToWorkspace bool) *ExecTool {
	t := 60
	if timeoutSeconds > 0 {
		t = timeoutSeconds
	}
	return &ExecTool{
		timeout:             time.Duration(t) * time.Second,
		workingDir:          workingDir,
		restrictToWorkspace: restrictToWorkspace,
	}
}

func (e *ExecTool) Name() string { return "exec" }
func (e *ExecTool) Description() string {
	return "Execute a shell command and return its output. Use with caution."
}
func (e *ExecTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {
				"type": "string",
				"description": "The shell command to execute"
			},
			"working_dir": {
				"type": "string",
				"description": "Optional working directory for the command"
			}
		},
		"required": ["command"]
	}`)
}

func (e *ExecTool) Execute(ctx context.Context, params map[string]any) (string, error) {
	command, _ := params["command"].(string)
	if command == "" {
		return "Error: command is required", nil
	}

	cwd := e.workingDir
	if wd, ok := params["working_dir"].(string); ok && wd != "" {
		cwd = wd
	}
	if cwd == "" {
		cwd, _ = os.Getwd()
	}

	if guard := e.guardCommand(command, cwd); guard != "" {
		return guard, nil
	}

	cmdCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "sh", "-c", command)
	cmd.Dir = cwd

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	var parts []string
	if out := stdout.String(); out != "" {
		parts = append(parts, out)
	}
	if errOut := stderr.String(); strings.TrimSpace(errOut) != "" {
		parts = append(parts, "STDERR:\n"+errOut)
	}
	if runErr != nil && cmd.ProcessState != nil && cmd.ProcessState.ExitCode() != 0 {
		parts = append(parts, fmt.Sprintf("\nExit code: %d", cmd.ProcessState.ExitCode()))
	}
	if cmdCtx.Err() != nil {
		return fmt.Sprintf("Error: Command timed out after %v", e.timeout), nil
	}

	result := strings.Join(parts, "\n")
	if result == "" {
		result = "(no output)"
	}
	const maxLen = 10000
	if len(result) > maxLen {
		result = result[:maxLen] + fmt.Sprintf("\n... (truncated, %d more chars)", len(result)-maxLen)
	}
	return result, nil
}

// guardCommand implements Python's _guard_command safety check.
func (e *ExecTool) guardCommand(command, cwd string) string {
	lower := strings.ToLower(strings.TrimSpace(command))

	for _, p := range denyPatterns {
		if p.MatchString(lower) {
			return "Error: Command blocked by safety guard (dangerous pattern detected)"
		}
	}

	if e.restrictToWorkspace {
		if strings.Contains(command, `..\\`) || strings.Contains(command, "../") {
			return "Error: Command blocked by safety guard (path traversal detected)"
		}

		cwdResolved, err := filepath.EvalSymlinks(cwd)
		if err != nil {
			cwdResolved = cwd
		}

		// Check absolute posix paths embedded in the command.
		posixPaths := extractAbsolutePaths(command)
		for _, raw := range posixPaths {
			p, err := filepath.EvalSymlinks(raw)
			if err != nil {
				p = filepath.Clean(raw)
			}
			if filepath.IsAbs(p) && !strings.HasPrefix(p, cwdResolved) && p != cwdResolved {
				return "Error: Command blocked by safety guard (path outside working dir)"
			}
		}
	}
	return ""
}

// extractAbsolutePaths extracts absolute path-like strings from a command line.
// Mirrors Python's posix_paths = re.findall(r"(?:^|[\s|>])(/[^\s\"'>]+)", cmd).
var absolutePathRE = regexp.MustCompile(`(?:^|[\s|>])(/[^\s"'>]+)`)

func extractAbsolutePaths(cmd string) []string {
	matches := absolutePathRE.FindAllStringSubmatch(cmd, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, strings.TrimSpace(m[1]))
	}
	return out
}
