package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nirhuri/dev-agent/internal/config"
	"github.com/nirhuri/dev-agent/internal/engine"
	"github.com/nirhuri/dev-agent/internal/repository"
	"github.com/nirhuri/dev-agent/internal/store"
	"github.com/nirhuri/dev-agent/internal/workflow"
)

func runValidateAndCommit(cmd *cobra.Command, cfg config.Config, db *store.SQLite, task *workflow.Task) error {
	if err := ensureTaskBranch(cmd, cfg, db, task); err != nil {
		return err
	}
	repo := task.Repository
	git := repository.NewGit(repo)
	ctx := cmd.Context()

	commands := cfg.Validation.Commands
	results, err := repository.RunValidation(ctx, repo, commands)
	if err != nil {
		return err
	}
	task.Validation = results
	_ = db.UpdateTask(ctx, task)

	for _, r := range results {
		status := "PASS"
		if !r.OK {
			status = "FAIL"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "[%s] %s\n", status, r.Command)
	}

	if !repository.AllOK(results) {
		if task.RepairAttempt >= cfg.Validation.RepairAttempts {
			_, _ = db.Transition(ctx, task.ID, workflow.StateFailed, "validation failed; repair limit reached")
			return fmt.Errorf("validation failed after %d repair attempts", task.RepairAttempt)
		}
		if _, err := db.Transition(ctx, task.ID, workflow.StateRepairing, "validation failed"); err != nil {
			return err
		}
		task.RepairAttempt++
		_ = db.UpdateTask(ctx, task)

		fmt.Fprintf(cmd.OutOrStdout(), "Repair attempt %d/%d via coding engine...\n", task.RepairAttempt, cfg.Validation.RepairAttempts)
		eng, err := newEngine(cfg)
		if err != nil {
			return err
		}
		var failOut strings.Builder
		for _, r := range results {
			if !r.OK {
				fmt.Fprintf(&failOut, "$ %s\n%s\n", r.Command, r.Output)
			}
		}
		_, err = eng.ImplementTask(ctx, engine.TaskRequest{
			IssueKey:    task.IssueKey,
			Summary:     task.IssueSummary,
			Plan:        task.Plan,
			Workspace:   repo,
			Autonomous:  cfg.Mode == "autonomous",
			ExtraPrompt: "Fix the failing validation without weakening or deleting tests:\n" + failOut.String(),
		})
		if err != nil {
			_, _ = db.Transition(ctx, task.ID, workflow.StateFailed, "repair engine failed")
			return err
		}
		if _, err := db.Transition(ctx, task.ID, workflow.StateValidating, "re-validate after repair"); err != nil {
			return err
		}
		return runValidateAndCommit(cmd, cfg, db, task)
	}

	files, err := git.DiffNameOnly(ctx)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No file changes to commit.")
		if _, err := db.Transition(ctx, task.ID, workflow.StateReadyForPR, "validated; no commits"); err != nil {
			return err
		}
		return nil
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Changed files:")
	for _, f := range files {
		fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", f)
	}
	secretHits := filterSecrets(files)
	if len(secretHits) > 0 {
		return fmt.Errorf("refusing to stage potential secrets: %s", strings.Join(secretHits, ", "))
	}

	if err := git.StageFiles(ctx, files); err != nil {
		return err
	}
	diff, _ := git.StagedDiff(ctx)
	if strings.TrimSpace(diff) == "" {
		return fmt.Errorf("nothing staged after add; aborting commit")
	}

	msg := fmt.Sprintf("feat: %s %s", task.IssueKey, shortSummary(task.IssueSummary))
	hash, err := git.Commit(ctx, msg)
	if err != nil {
		return err
	}
	task.CommitHashes = append(task.CommitHashes, hash)
	if err := db.UpdateTask(ctx, task); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Committed %s\n", hash[:min(12, len(hash))])

	if _, err := db.Transition(ctx, task.ID, workflow.StateReadyForPR, "validated and committed"); err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), "State -> READY_FOR_PR")
	return nil
}

func filterSecrets(files []string) []string {
	var hits []string
	for _, f := range files {
		base := strings.ToLower(filepath.Base(f))
		switch {
		case base == ".env", strings.HasSuffix(base, ".pem"), strings.HasSuffix(base, ".key"),
			base == "credentials.json", base == "id_rsa", strings.Contains(base, "secret"):
			hits = append(hits, f)
		}
	}
	return hits
}

func shortSummary(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "agent changes"
	}
	if len(s) > 60 {
		return s[:60]
	}
	return s
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
