package cli

import (
	"bufio"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/nirhuri/dev-agent/internal/store"
	"github.com/nirhuri/dev-agent/internal/workflow"
)

func newStatusCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show current and resumable tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openStore()
			if err != nil {
				return err
			}
			defer db.Close()

			svc := &workflow.Service{Store: db}
			tasks, err := svc.Status(cmd.Context(), all)
			if err != nil {
				return err
			}
			if len(tasks) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No tasks found.")
				return nil
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tISSUE\tSTATE\tREPO\tUPDATED")
			for _, t := range tasks {
				repo := t.Repository
				if len(repo) > 40 {
					repo = "..." + repo[len(repo)-37:]
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					shortID(t.ID), t.IssueKey, t.State, orDash(repo), t.UpdatedAt.Local().Format(time.RFC822))
			}
			return w.Flush()
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "include completed and cancelled tasks")
	return cmd
}

func newResumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume [task-id|issue-key]",
		Short: "Resume a paused or interrupted task",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openStore()
			if err != nil {
				return err
			}
			defer db.Close()

			svc := &workflow.Service{Store: db, Owner: "cli"}
			idOrKey := ""
			if len(args) == 1 {
				idOrKey = args[0]
			} else {
				tasks, err := svc.Status(cmd.Context(), false)
				if err != nil {
					return err
				}
				if len(tasks) == 0 {
					return fmt.Errorf("no active tasks to resume")
				}
				if len(tasks) > 1 {
					fmt.Fprintln(cmd.OutOrStdout(), "Multiple active tasks; specify an id or issue key:")
					for _, t := range tasks {
						fmt.Fprintf(cmd.OutOrStdout(), "  %s  %s  %s\n", shortID(t.ID), t.IssueKey, t.State)
					}
					return fmt.Errorf("ambiguous resume target")
				}
				idOrKey = tasks[0].ID
			}

			task, err := svc.Resume(cmd.Context(), idOrKey)
			if err != nil {
				return err
			}
			defer func() { _ = svc.Release(cmd.Context(), task.ID) }()

			fmt.Fprintf(cmd.OutOrStdout(), "Resumed %s (%s) in state %s\n", task.IssueKey, shortID(task.ID), task.State)
			if task.State == workflow.StateWaitingClarification {
				return runAnswerClarifications(cmd, svc, task)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Task is ready. Full workflow execution arrives in later milestones.")
			fmt.Fprintln(cmd.OutOrStdout(), "Repository/config/credential revalidation will run before implementation steps.")
			return nil
		},
	}
}

func runAnswerClarifications(cmd *cobra.Command, svc *workflow.Service, task *workflow.Task) error {
	fmt.Fprintln(cmd.OutOrStdout(), "Clarification questions:")
	reader := bufio.NewReader(cmd.InOrStdin())
	var answers []string
	for i, qa := range task.Clarifications {
		if qa.Answer != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "  %d. %s\n     (answered: %s)\n", i+1, qa.Question, qa.Answer)
			continue
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  %d. %s\n", i+1, qa.Question)
		fmt.Fprint(cmd.OutOrStdout(), "     answer: ")
		line, err := readLine(reader)
		if err != nil {
			return err
		}
		answers = append(answers, line)
	}
	if len(answers) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No unanswered questions.")
		return nil
	}
	updated, err := svc.AnswerClarifications(cmd.Context(), task.ID, answers)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Answers saved. Task is now %s.\n", updated.State)
	return nil
}

func openStore() (*store.SQLite, error) {
	return store.OpenDefault()
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}
