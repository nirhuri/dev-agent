package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nirhuri/dev-agent/internal/config"
	"github.com/nirhuri/dev-agent/internal/secrets"
)

func newSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "First-time setup wizard",
		Long:  "Interactively configure Jira, coding engine, and defaults. Secrets are stored in the OS keyring.",
		RunE: func(cmd *cobra.Command, args []string) error {
			configFile, _ := cmd.Flags().GetString("config")
			return runSetup(cmd.InOrStdin(), cmd.OutOrStdout(), configFile)
		},
	}
}

func runSetup(in io.Reader, out io.Writer, configFile string) error {
	reader := bufio.NewReader(in)
	cfg := config.Default()

	if exists, _ := config.Exists(configFile); exists {
		loaded, _, err := config.Load(config.LoadOptions{ConfigFile: configFile})
		if err == nil {
			cfg = loaded
			fmt.Fprintln(out, "Existing configuration found; values shown as defaults.")
		}
	}

	fmt.Fprintln(out, "Dev Agent setup")
	fmt.Fprintln(out, "Press Enter to keep a default. Optional steps can be skipped.")
	fmt.Fprintln(out)

	mode, err := prompt(reader, out, "Operating mode (interactive/autonomous)", cfg.Mode)
	if err != nil {
		return err
	}
	cfg.Mode = strings.TrimSpace(mode)

	fmt.Fprintln(out)
	fmt.Fprintln(out, "--- Jira (optional; skip to configure later) ---")
	baseURL, err := prompt(reader, out, "Jira base URL (e.g. https://example.atlassian.net)", cfg.Jira.BaseURL)
	if err != nil {
		return err
	}
	cfg.Jira.BaseURL = strings.TrimSpace(baseURL)

	if cfg.Jira.BaseURL != "" {
		email, err := prompt(reader, out, "Jira email", cfg.Jira.Email)
		if err != nil {
			return err
		}
		cfg.Jira.Email = strings.TrimSpace(email)

		project, err := prompt(reader, out, "Default project key", cfg.Jira.ProjectKey)
		if err != nil {
			return err
		}
		cfg.Jira.ProjectKey = strings.TrimSpace(project)

		jql, err := prompt(reader, out, "Default JQL (empty = project filter)", cfg.Jira.JQL)
		if err != nil {
			return err
		}
		cfg.Jira.JQL = strings.TrimSpace(jql)

		token, err := promptSecret(reader, out, "Jira API token (stored in keyring; leave empty to skip)")
		if err != nil {
			return err
		}
		if token != "" {
			if err := secrets.Store(secrets.KeyJiraToken, token); err != nil {
				return fmt.Errorf("store Jira token: %w", err)
			}
			fmt.Fprintln(out, "Jira token stored in keyring.")
		}
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "--- Coding engine ---")
	provider, err := prompt(reader, out, "Engine provider", cfg.Engine.Provider)
	if err != nil {
		return err
	}
	cfg.Engine.Provider = strings.TrimSpace(provider)

	agentPath, err := prompt(reader, out, "Cursor agent CLI path", cfg.Engine.AgentPath)
	if err != nil {
		return err
	}
	cfg.Engine.AgentPath = strings.TrimSpace(agentPath)

	apiKey, err := promptSecret(reader, out, "Cursor API key (optional; stored in keyring)")
	if err != nil {
		return err
	}
	if apiKey != "" {
		if err := secrets.Store(secrets.KeyCursorAPIKey, apiKey); err != nil {
			return fmt.Errorf("store Cursor API key: %w", err)
		}
		fmt.Fprintln(out, "Cursor API key stored in keyring.")
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "--- Repository (optional) ---")
	cwd, _ := os.Getwd()
	repoPath, err := prompt(reader, out, "Local repository path to register", cwd)
	if err != nil {
		return err
	}
	repoPath = strings.TrimSpace(repoPath)
	if repoPath != "" {
		abs, err := filepath.Abs(repoPath)
		if err != nil {
			return err
		}
		name := filepath.Base(abs)
		nameIn, err := prompt(reader, out, "Repository name", name)
		if err != nil {
			return err
		}
		cfg.Repos = upsertRepo(cfg.Repos, config.RepoConfig{
			Name: strings.TrimSpace(nameIn),
			Path: abs,
		})
	}

	fmt.Fprintln(out)
	attemptsStr, err := prompt(reader, out, "Max repair attempts", strconv.Itoa(cfg.Validation.RepairAttempts))
	if err != nil {
		return err
	}
	if n, err := strconv.Atoi(strings.TrimSpace(attemptsStr)); err == nil {
		cfg.Validation.RepairAttempts = n
	}

	path := configFile
	if path == "" {
		path, err = config.FilePath()
		if err != nil {
			return err
		}
	}

	if err := config.Save(cfg, path); err != nil {
		return err
	}

	fmt.Fprintln(out)
	fmt.Fprintf(out, "Configuration saved to %s\n", path)
	fmt.Fprintln(out, "Secrets are in the OS keyring, not the config file.")
	fmt.Fprintln(out, "Run `dev-agent doctor` next, or `dev-agent config` to change settings later.")
	return nil
}

func prompt(reader *bufio.Reader, out io.Writer, label, def string) (string, error) {
	if def != "" {
		fmt.Fprintf(out, "%s [%s]: ", label, def)
	} else {
		fmt.Fprintf(out, "%s: ", label)
	}
	line, err := reader.ReadString('\n')
	if err != nil && len(line) == 0 {
		return "", err
	}
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return def, nil
	}
	return line, nil
}

func promptSecret(reader *bufio.Reader, out io.Writer, label string) (string, error) {
	fmt.Fprintf(out, "%s: ", label)
	line, err := reader.ReadString('\n')
	if err != nil && len(line) == 0 {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func upsertRepo(repos []config.RepoConfig, r config.RepoConfig) []config.RepoConfig {
	for i, existing := range repos {
		if existing.Name == r.Name || existing.Path == r.Path {
			repos[i] = r
			return repos
		}
	}
	return append(repos, r)
}
