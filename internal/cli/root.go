package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/nirhuri/dev-agent/internal/config"
)

// Version is set at build time via -ldflags when available.
var Version = "0.1.0-dev"

// Execute runs the root command.
func Execute() error {
	return NewRootCmd(os.Stdout, os.Stderr).Execute()
}

// NewRootCmd builds the command tree. Writers are injectable for tests.
func NewRootCmd(stdout, stderr io.Writer) *cobra.Command {
	var configFile string

	root := &cobra.Command{
		Use:           "dev-agent",
		Short:         "Personal AI software-engineering agent",
		Long:          "dev-agent takes Jira work items through planning, implementation, validation, and pull requests.",
		SilenceErrors: true,
		SilenceUsage:  true,
		Version:       Version,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cmd.SetOut(stdout)
			cmd.SetErr(stderr)
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRoot(cmd, configFile)
		},
	}

	root.PersistentFlags().StringVar(&configFile, "config", "", "path to config file (default: ~/.config/dev-agent/config.yaml)")
	root.SetOut(stdout)
	root.SetErr(stderr)

	root.AddCommand(newSetupCmd())
	root.AddCommand(newConfigCmd())
	root.AddCommand(newDoctorCmd())

	return root
}

func runRoot(cmd *cobra.Command, configFile string) error {
	exists, err := config.Exists(configFile)
	if err != nil {
		return err
	}
	if !exists {
		fmt.Fprintln(cmd.OutOrStdout(), "Welcome to Dev Agent")
		fmt.Fprintln(cmd.OutOrStdout())
		fmt.Fprintln(cmd.OutOrStdout(), "No configuration found. Run `dev-agent setup` to get started.")
		fmt.Fprintln(cmd.OutOrStdout(), "Then use `dev-agent doctor` to validate your environment.")
		return nil
	}

	cfg, source, err := config.Load(config.LoadOptions{ConfigFile: configFile})
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Welcome to Dev Agent")
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintf(cmd.OutOrStdout(), "Config: %s\n", source)
	fmt.Fprintf(cmd.OutOrStdout(), "Mode: %s\n", cfg.Mode)
	fmt.Fprintf(cmd.OutOrStdout(), "Coding engine: %s\n", cfg.Engine.Provider)
	if cfg.JiraConfigured() {
		fmt.Fprintf(cmd.OutOrStdout(), "Jira: %s\n", cfg.Jira.BaseURL)
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "Jira: not configured")
	}
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "Next: `dev-agent issues` (coming soon) or `dev-agent doctor`.")
	return nil
}

func loadConfigFromCmd(cmd *cobra.Command) (config.Config, string, error) {
	configFile, _ := cmd.Flags().GetString("config")
	return config.Load(config.LoadOptions{ConfigFile: configFile})
}
