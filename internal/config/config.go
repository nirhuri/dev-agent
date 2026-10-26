package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// Version is the configuration schema version.
	Version = 1

	AppName = "dev-agent"

	DefaultMode           = "interactive"
	DefaultRepairAttempts = 3
	DefaultJiraMaxResults = 50
	DefaultEngine         = "cursor"
	DefaultPRProvider     = "github"
)

// Config is the validated application configuration.
type Config struct {
	Version    int              `mapstructure:"version" yaml:"version"`
	Mode       string           `mapstructure:"mode" yaml:"mode"`
	Jira       JiraConfig       `mapstructure:"jira" yaml:"jira"`
	Engine     EngineConfig     `mapstructure:"engine" yaml:"engine"`
	Git        GitConfig        `mapstructure:"git" yaml:"git"`
	Validation ValidationConfig `mapstructure:"validation" yaml:"validation"`
	Repos      []RepoConfig     `mapstructure:"repositories" yaml:"repositories"`
}

// JiraConfig holds Jira Cloud connection settings (no secrets).
type JiraConfig struct {
	BaseURL    string `mapstructure:"base_url" yaml:"base_url"`
	Email      string `mapstructure:"email" yaml:"email"`
	ProjectKey string `mapstructure:"project_key" yaml:"project_key"`
	JQL        string `mapstructure:"jql" yaml:"jql"`
	MaxResults int    `mapstructure:"max_results" yaml:"max_results"`
}

// EngineConfig selects and configures the coding engine.
type EngineConfig struct {
	Provider  string `mapstructure:"provider" yaml:"provider"`
	AgentPath string `mapstructure:"agent_path" yaml:"agent_path"`
	Model     string `mapstructure:"model" yaml:"model"`
}

// GitConfig holds Git and PR defaults.
type GitConfig struct {
	ProtectedBranches []string `mapstructure:"protected_branches" yaml:"protected_branches"`
	BranchPrefix      string   `mapstructure:"branch_prefix" yaml:"branch_prefix"`
	PRProvider        string   `mapstructure:"pr_provider" yaml:"pr_provider"`
}

// ValidationConfig controls test/lint/repair behaviour.
type ValidationConfig struct {
	Commands       []string `mapstructure:"commands" yaml:"commands"`
	RepairAttempts int      `mapstructure:"repair_attempts" yaml:"repair_attempts"`
}

// RepoConfig is a known local repository.
type RepoConfig struct {
	Name string `mapstructure:"name" yaml:"name"`
	Path string `mapstructure:"path" yaml:"path"`
}

// Default returns a Config with safe defaults.
func Default() Config {
	return Config{
		Version: Version,
		Mode:    DefaultMode,
		Jira: JiraConfig{
			MaxResults: DefaultJiraMaxResults,
		},
		Engine: EngineConfig{
			Provider:  DefaultEngine,
			AgentPath: "agent",
		},
		Git: GitConfig{
			ProtectedBranches: []string{"main", "master"},
			BranchPrefix:      "agent/",
			PRProvider:        DefaultPRProvider,
		},
		Validation: ValidationConfig{
			RepairAttempts: DefaultRepairAttempts,
		},
	}
}

// Validate checks required fields and value constraints.
func (c Config) Validate() error {
	var errs []string

	if c.Version < 1 {
		errs = append(errs, "version must be >= 1")
	}
	if c.Mode != "interactive" && c.Mode != "autonomous" {
		errs = append(errs, "mode must be 'interactive' or 'autonomous'")
	}
	if c.Jira.BaseURL != "" {
		if !strings.HasPrefix(c.Jira.BaseURL, "https://") && !strings.HasPrefix(c.Jira.BaseURL, "http://") {
			errs = append(errs, "jira.base_url must start with http:// or https://")
		}
	}
	if c.Jira.MaxResults < 1 || c.Jira.MaxResults > 100 {
		errs = append(errs, "jira.max_results must be between 1 and 100")
	}
	if c.Engine.Provider == "" {
		errs = append(errs, "engine.provider is required")
	}
	if c.Validation.RepairAttempts < 0 {
		errs = append(errs, "validation.repair_attempts must be >= 0")
	}
	if c.Git.PRProvider != "" && c.Git.PRProvider != "github" {
		errs = append(errs, "git.pr_provider must be 'github' for MVP")
	}
	for i, r := range c.Repos {
		if r.Name == "" {
			errs = append(errs, fmt.Sprintf("repositories[%d].name is required", i))
		}
		if r.Path == "" {
			errs = append(errs, fmt.Sprintf("repositories[%d].path is required", i))
		}
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// JiraConfigured returns true when enough Jira settings exist to attempt API calls
// (token is stored separately in the keyring).
func (c Config) JiraConfigured() bool {
	return c.Jira.BaseURL != "" && c.Jira.Email != ""
}

// Dir returns the configuration directory (~/.config/dev-agent).
func Dir() (string, error) {
	if override := os.Getenv("DEV_AGENT_CONFIG_DIR"); override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".config", AppName), nil
}

// FilePath returns the path to the main config file.
func FilePath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// DataDir returns the local data directory for SQLite and task state.
func DataDir() (string, error) {
	if override := os.Getenv("DEV_AGENT_DATA_DIR"); override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".local", "share", AppName), nil
}

// Redacted returns a copy safe for display (no secret values; only presence hints).
func (c Config) Redacted() map[string]any {
	return map[string]any{
		"version": c.Version,
		"mode":    c.Mode,
		"jira": map[string]any{
			"base_url":    c.Jira.BaseURL,
			"email":       c.Jira.Email,
			"project_key": c.Jira.ProjectKey,
			"jql":         c.Jira.JQL,
			"max_results": c.Jira.MaxResults,
			"token":       "[stored in keyring]",
		},
		"engine": map[string]any{
			"provider":   c.Engine.Provider,
			"agent_path": c.Engine.AgentPath,
			"model":      c.Engine.Model,
			"api_key":    "[stored in keyring or env]",
		},
		"git": map[string]any{
			"protected_branches": c.Git.ProtectedBranches,
			"branch_prefix":      c.Git.BranchPrefix,
			"pr_provider":        c.Git.PRProvider,
		},
		"validation": map[string]any{
			"commands":        c.Validation.Commands,
			"repair_attempts": c.Validation.RepairAttempts,
		},
		"repositories": c.Repos,
	}
}
