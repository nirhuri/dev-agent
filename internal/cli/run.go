package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/nirhuri/dev-agent/internal/workflow"
)

func newRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run ISSUE-KEY",
		Short: "Start or resume work on a Jira issue",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := loadConfigFromCmd(cmd)
			if err != nil {
				return err
			}
			db, err := openStore()
			if err != nil {
				return err
			}
			defer db.Close()

			repo := ""
			if cwd, err := os.Getwd(); err == nil {
				repo = cwd
			}
			if len(cfg.Repos) == 1 {
				repo = cfg.Repos[0].Path
			}

			summary := ""
			if cfg.JiraConfigured() {
				client, err := newJiraClient(cfg)
				if err == nil {
					detail, err := client.GetIssue(cmd.Context(), args[0])
					if err == nil {
						summary = detail.Summary
						fmt.Fprintf(cmd.OutOrStdout(), "Loaded %s — %s\n", detail.Key, detail.Summary)
					} else {
						fmt.Fprintf(cmd.OutOrStdout(), "Warning: could not fetch issue details: %v\n", err)
					}
				}
			}

			svc := &workflow.Service{Store: db, Owner: "cli"}
			task, err := svc.StartFromIssue(cmd.Context(), args[0], summary, repo, cfg.Engine.Provider)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Task %s · %s · state %s\n", shortID(task.ID), task.IssueKey, task.State)
			if repo != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Repository: %s\n", filepath.Clean(repo))
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Use `dev-agent status` to inspect and `dev-agent resume` to continue.")
			fmt.Fprintln(cmd.OutOrStdout(), "Analysis/implementation engines are wired in later milestones.")
			return nil
		},
	}
}
