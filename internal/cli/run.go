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

			eng, err := newEngine(cfg)
			if err != nil {
				return err
			}
			cap, err := eng.CheckAvailability(cmd.Context())
			if err != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Coding engine unavailable: %v\n", err)
				fmt.Fprintln(cmd.OutOrStdout(), "Task is persisted; fix the engine then `dev-agent resume`.")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Coding engine: %s (%s)\n", cap.Provider, cap.AutomationLevel)

			if task.State == workflow.StateIssueSelected {
				if _, err := db.Transition(cmd.Context(), task.ID, workflow.StateAnalyzing, "begin analysis"); err != nil {
					return err
				}
				task.Plan = fmt.Sprintf("Implement %s: %s", task.IssueKey, task.IssueSummary)
				if err := db.UpdateTask(cmd.Context(), task); err != nil {
					return err
				}
				if _, err := db.Transition(cmd.Context(), task.ID, workflow.StatePlanReady, "plan drafted"); err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), "Plan ready. Use `dev-agent resume` to implement with the coding engine.")
			}
			return nil
		},
	}
}
