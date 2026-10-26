package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nirhuri/dev-agent/internal/config"
	"github.com/nirhuri/dev-agent/internal/engine"
	"github.com/nirhuri/dev-agent/internal/store"
	"github.com/nirhuri/dev-agent/internal/workflow"
)

func runImplement(cmd *cobra.Command, cfg config.Config, db *store.SQLite, task *workflow.Task) error {
	eng, err := newEngine(cfg)
	if err != nil {
		return err
	}
	cap, err := eng.CheckAvailability(cmd.Context())
	if err != nil {
		return fmt.Errorf("coding engine unavailable: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Implementing with %s (%s)...\n", cap.Provider, cap.AutomationLevel)

	if task.State == workflow.StatePlanReady {
		if _, err := db.Transition(cmd.Context(), task.ID, workflow.StateImplementing, "engine start"); err != nil {
			return err
		}
	}

	var clarifications []string
	for _, qa := range task.Clarifications {
		if qa.Answer != "" {
			clarifications = append(clarifications, qa.Question+": "+qa.Answer)
		}
	}

	req := engine.TaskRequest{
		IssueKey:       task.IssueKey,
		Summary:        task.IssueSummary,
		Plan:           task.Plan,
		Clarifications: clarifications,
		Workspace:      task.Repository,
		Model:          cfg.Engine.Model,
		Autonomous:     cfg.Mode == "autonomous",
	}

	result, err := eng.ImplementTask(cmd.Context(), req)
	if err != nil {
		task.ErrorMessage = err.Error()
		_ = db.UpdateTask(cmd.Context(), task)
		_, _ = db.Transition(cmd.Context(), task.ID, workflow.StateFailed, "engine failed")
		return err
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Engine finished:")
	if result.Summary != "" {
		fmt.Fprintln(cmd.OutOrStdout(), result.Summary)
	} else if result.Output != "" {
		fmt.Fprintln(cmd.OutOrStdout(), truncateRunes(result.Output, 1000))
	}

	// Do not claim files changed without verification — leave state for validation milestone.
	task.ErrorMessage = ""
	if err := db.UpdateTask(cmd.Context(), task); err != nil {
		return err
	}
	if _, err := db.Transition(cmd.Context(), task.ID, workflow.StateValidating, "engine completed; validate next"); err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), "State -> VALIDATING. Git/validation steps arrive in the next milestone.")
	fmt.Fprintln(cmd.OutOrStdout(), "Note: engine exit success is not proof of a correct change; validation will verify.")
	return nil
}
