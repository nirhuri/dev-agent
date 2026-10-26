package doctor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/nirhuri/dev-agent/internal/config"
	"github.com/nirhuri/dev-agent/internal/secrets"
)

// Severity classifies a check result.
type Severity string

const (
	SeverityOK      Severity = "ok"
	SeverityWarn    Severity = "warn"
	SeverityError   Severity = "error"
	SeveritySkipped Severity = "skipped"
)

// Result is one diagnostic check.
type Result struct {
	Name     string
	Severity Severity
	Message  string
	Fix      string
}

// Report is the full doctor output.
type Report struct {
	Results []Result
}

// HasErrors reports whether any check failed.
func (r Report) HasErrors() bool {
	for _, res := range r.Results {
		if res.Severity == SeverityError {
			return true
		}
	}
	return false
}

// Run executes environment and configuration checks.
func Run(ctx context.Context, cfg config.Config, configSource string) Report {
	var results []Result

	results = append(results, checkConfig(cfg, configSource))
	results = append(results, checkGit(ctx))
	results = append(results, checkGH(ctx))
	results = append(results, checkJira(cfg))
	results = append(results, checkCursorAgent(ctx, cfg))
	results = append(results, checkDataDir())

	return Report{Results: results}
}

func checkConfig(cfg config.Config, source string) Result {
	if err := cfg.Validate(); err != nil {
		return Result{
			Name:     "config",
			Severity: SeverityError,
			Message:  err.Error(),
			Fix:      "Run `dev-agent setup` or `dev-agent config` to fix settings.",
		}
	}
	return Result{
		Name:     "config",
		Severity: SeverityOK,
		Message:  fmt.Sprintf("valid (source: %s)", source),
	}
}

func checkGit(ctx context.Context) Result {
	path, err := exec.LookPath("git")
	if err != nil {
		return Result{
			Name:     "git",
			Severity: SeverityError,
			Message:  "git not found on PATH",
			Fix:      "Install Git and ensure it is available on PATH.",
		}
	}
	out, err := runCmd(ctx, path, "--version")
	if err != nil {
		return Result{
			Name:     "git",
			Severity: SeverityError,
			Message:  err.Error(),
			Fix:      "Repair your Git installation.",
		}
	}
	return Result{
		Name:     "git",
		Severity: SeverityOK,
		Message:  strings.TrimSpace(out),
	}
}

func checkGH(ctx context.Context) Result {
	path, err := exec.LookPath("gh")
	if err != nil {
		return Result{
			Name:     "gh",
			Severity: SeverityWarn,
			Message:  "GitHub CLI not found (required for PR creation)",
			Fix:      "Install gh: https://cli.github.com/",
		}
	}
	out, err := runCmd(ctx, path, "auth", "status")
	if err != nil {
		return Result{
			Name:     "gh",
			Severity: SeverityWarn,
			Message:  "gh installed but not authenticated",
			Fix:      "Run `gh auth login`.",
		}
	}
	// Avoid dumping full auth status (may include account details); keep short.
	_ = out
	return Result{
		Name:     "gh",
		Severity: SeverityOK,
		Message:  "authenticated",
	}
}

func checkJira(cfg config.Config) Result {
	if !cfg.JiraConfigured() {
		return Result{
			Name:     "jira",
			Severity: SeverityWarn,
			Message:  "not configured",
			Fix:      "Run `dev-agent setup` to set Jira base URL and email.",
		}
	}
	tokenSet := secrets.Exists(secrets.KeyJiraToken)
	if !tokenSet {
		return Result{
			Name:     "jira",
			Severity: SeverityWarn,
			Message:  fmt.Sprintf("URL/email set; token %s", secrets.Status(secrets.KeyJiraToken)),
			Fix:      "Store a Jira API token via `dev-agent setup` or `dev-agent config`.",
		}
	}
	return Result{
		Name:     "jira",
		Severity: SeverityOK,
		Message:  fmt.Sprintf("%s configured; token %s", cfg.Jira.BaseURL, secrets.Status(secrets.KeyJiraToken)),
	}
}

func checkCursorAgent(ctx context.Context, cfg config.Config) Result {
	path := cfg.Engine.AgentPath
	if path == "" {
		path = "agent"
	}
	resolved, err := exec.LookPath(path)
	if err != nil {
		return Result{
			Name:     "cursor-agent",
			Severity: SeverityWarn,
			Message:  fmt.Sprintf("%q not found on PATH", path),
			Fix:      "Install the Cursor Agent CLI and ensure `agent` is on PATH.",
		}
	}
	out, err := runCmd(ctx, resolved, "--version")
	if err != nil {
		return Result{
			Name:     "cursor-agent",
			Severity: SeverityWarn,
			Message:  err.Error(),
			Fix:      "Repair the Cursor Agent CLI installation.",
		}
	}
	msg := fmt.Sprintf("%s (%s)", strings.TrimSpace(out), resolved)
	if os.Getenv("CURSOR_API_KEY") != "" || secrets.Exists(secrets.KeyCursorAPIKey) {
		msg += "; API key available"
	} else {
		msg += "; API key not set (login or CURSOR_API_KEY may still work)"
	}
	return Result{
		Name:     "cursor-agent",
		Severity: SeverityOK,
		Message:  msg,
	}
}

func checkDataDir() Result {
	dir, err := config.DataDir()
	if err != nil {
		return Result{
			Name:     "data-dir",
			Severity: SeverityError,
			Message:  err.Error(),
			Fix:      "Set DEV_AGENT_DATA_DIR or fix home directory permissions.",
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Result{
			Name:     "data-dir",
			Severity: SeverityError,
			Message:  err.Error(),
			Fix:      "Ensure the data directory is writable.",
		}
	}
	return Result{
		Name:     "data-dir",
		Severity: SeverityOK,
		Message:  dir,
	}
}

func runCmd(ctx context.Context, name string, args ...string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
