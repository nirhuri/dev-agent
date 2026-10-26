package repository

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/nirhuri/dev-agent/internal/workflow"
)

// DetectValidationCommands inspects the repository for common check commands.
func DetectValidationCommands(dir string) []string {
	var cmds []string
	if exists(filepath.Join(dir, "go.mod")) {
		cmds = append(cmds, "go test ./...")
	}
	if exists(filepath.Join(dir, "package.json")) {
		// Prefer npm test if package.json exists; do not invent scripts.
		cmds = append(cmds, "npm test --if-present")
	}
	if exists(filepath.Join(dir, "pyproject.toml")) || exists(filepath.Join(dir, "pytest.ini")) {
		cmds = append(cmds, "python -m pytest")
	}
	if exists(filepath.Join(dir, "Cargo.toml")) {
		cmds = append(cmds, "cargo test")
	}
	if exists(filepath.Join(dir, "Makefile")) {
		// Only add make test if we cannot detect a language-specific default;
		// keep as additional hint when Makefile exists alone.
		if len(cmds) == 0 {
			cmds = append(cmds, "make test")
		}
	}
	return cmds
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// RunValidation executes validation commands and returns structured results.
func RunValidation(ctx context.Context, dir string, commands []string) ([]workflow.ValidationResult, error) {
	if len(commands) == 0 {
		commands = DetectValidationCommands(dir)
	}
	if len(commands) == 0 {
		return []workflow.ValidationResult{{
			Command: "(none detected)",
			OK:      false,
			Output:  "no validation commands configured or detected",
		}}, nil
	}
	var results []workflow.ValidationResult
	for _, command := range commands {
		res := runOne(ctx, dir, command)
		results = append(results, res)
	}
	return results, nil
}

func runOne(ctx context.Context, dir, command string) workflow.ValidationResult {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(cctx, "sh", "-c", command)
	cmd.Dir = dir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	res := workflow.ValidationResult{
		Command: command,
		Output:  truncate(buf.String(), 8000),
		OK:      err == nil,
	}
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			res.ExitCode = ee.ExitCode()
		} else {
			res.ExitCode = -1
			res.Output = strings.TrimSpace(res.Output + "\n" + err.Error())
		}
	}
	return res
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + fmt.Sprintf("\n... truncated (%d bytes total)", len(s))
}

// AllOK reports whether every validation result succeeded.
func AllOK(results []workflow.ValidationResult) bool {
	if len(results) == 0 {
		return false
	}
	for _, r := range results {
		if !r.OK {
			return false
		}
	}
	return true
}
