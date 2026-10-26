package cli

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"

	"github.com/nirhuri/dev-agent/internal/config"
	"github.com/nirhuri/dev-agent/internal/secrets"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect or update configuration",
		Long:  "Show redacted configuration, or update individual settings without re-running full setup.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigShow(cmd)
		},
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Print redacted configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigShow(cmd)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value",
		Long: `Set a non-secret configuration key.

Supported keys:
  mode
  jira.base_url
  jira.email
  jira.project_key
  jira.jql
  jira.max_results
  engine.provider
  engine.agent_path
  engine.model
  git.pr_provider
  git.branch_prefix
  validation.repair_attempts

Secrets (jira token, cursor API key) use: dev-agent config set-secret <name>`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigSet(cmd, args[0], args[1])
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "set-secret <name>",
		Short: "Store a secret in the OS keyring",
		Long:  "Names: jira_token, cursor_api_key. Value is read from a prompt (not argv).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigSetSecret(cmd, args[0])
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "path",
		Short: "Print the configuration file path",
		RunE: func(cmd *cobra.Command, args []string) error {
			configFile, _ := cmd.Flags().GetString("config")
			if configFile != "" {
				fmt.Fprintln(cmd.OutOrStdout(), configFile)
				return nil
			}
			path, err := config.FilePath()
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), path)
			return nil
		},
	})

	return cmd
}

func runConfigShow(cmd *cobra.Command) error {
	cfg, source, err := loadConfigFromCmd(cmd)
	if err != nil {
		// Allow show when file missing: print defaults.
		if exists, _ := config.Exists(""); !exists {
			cfg = config.Default()
			source = "defaults (no config file)"
		} else {
			return err
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "# source: %s\n", source)
	fmt.Fprintf(cmd.OutOrStdout(), "# secrets: jira_token=%s cursor_api_key=%s\n",
		secrets.Status(secrets.KeyJiraToken),
		secrets.Status(secrets.KeyCursorAPIKey),
	)
	data, err := yaml.Marshal(cfg.Redacted())
	if err != nil {
		return err
	}
	fmt.Fprint(cmd.OutOrStdout(), string(data))
	return nil
}

func runConfigSet(cmd *cobra.Command, key, value string) error {
	configFile, _ := cmd.Flags().GetString("config")
	cfg, _, err := config.Load(config.LoadOptions{ConfigFile: configFile})
	if err != nil {
		exists, exErr := config.Exists(configFile)
		if exErr != nil {
			return exErr
		}
		if !exists {
			cfg = config.Default()
		} else {
			return err
		}
	}

	if err := applyConfigKey(&cfg, key, value); err != nil {
		return err
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
	fmt.Fprintf(cmd.OutOrStdout(), "Updated %s\n", key)
	return nil
}

func applyConfigKey(cfg *config.Config, key, value string) error {
	switch key {
	case "mode":
		cfg.Mode = value
	case "jira.base_url":
		cfg.Jira.BaseURL = value
	case "jira.email":
		cfg.Jira.Email = value
	case "jira.project_key":
		cfg.Jira.ProjectKey = value
	case "jira.jql":
		cfg.Jira.JQL = value
	case "jira.max_results":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("jira.max_results must be an integer")
		}
		cfg.Jira.MaxResults = n
	case "engine.provider":
		cfg.Engine.Provider = value
	case "engine.agent_path":
		cfg.Engine.AgentPath = value
	case "engine.model":
		cfg.Engine.Model = value
	case "git.pr_provider":
		cfg.Git.PRProvider = value
	case "git.branch_prefix":
		cfg.Git.BranchPrefix = value
	case "validation.repair_attempts":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("validation.repair_attempts must be an integer")
		}
		cfg.Validation.RepairAttempts = n
	default:
		return fmt.Errorf("unknown key %q; see `dev-agent config set --help`", key)
	}
	return nil
}

func runConfigSetSecret(cmd *cobra.Command, name string) error {
	var key string
	switch name {
	case secrets.KeyJiraToken, "jira":
		key = secrets.KeyJiraToken
	case secrets.KeyCursorAPIKey, "cursor":
		key = secrets.KeyCursorAPIKey
	default:
		return fmt.Errorf("unknown secret %q; use jira_token or cursor_api_key", name)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Enter value for %s: ", key)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && len(line) == 0 {
		return err
	}
	value := strings.TrimRight(line, "\r\n")
	if value == "" {
		return fmt.Errorf("empty value; secret not changed")
	}
	if err := secrets.Store(key, value); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Stored %s in keyring.\n", key)
	return nil
}
