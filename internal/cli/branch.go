package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nirhuri/dev-agent/internal/config"
	"github.com/nirhuri/dev-agent/internal/repository"
	"github.com/nirhuri/dev-agent/internal/store"
	"github.com/nirhuri/dev-agent/internal/workflow"
)

// ensureTaskBranch moves work onto a non-protected task branch when needed.
func ensureTaskBranch(cmd *cobra.Command, cfg config.Config, db *store.SQLite, task *workflow.Task) error {
	if task.Repository == "" {
		return fmt.Errorf("task has no repository path")
	}
	git := repository.NewGit(task.Repository)
	ctx := cmd.Context()
	if err := git.EnsureRepo(ctx); err != nil {
		return err
	}

	branch, err := git.CurrentBranch(ctx)
	if err != nil {
		return err
	}

	if task.Branch != "" && branch == task.Branch {
		return nil
	}

	if repository.IsProtectedBranch(branch, cfg.Git.ProtectedBranches) {
		dirty, err := git.IsDirty(ctx)
		if err != nil {
			return err
		}
		if dirty {
			return fmt.Errorf("working tree is dirty on protected branch %q; commit/stash/resolve manually before the agent continues", branch)
		}
		name := task.Branch
		if name == "" {
			name = repository.SanitizeBranchSlug(cfg.Git.BranchPrefix, task.IssueKey, task.IssueSummary)
		}
		exists, err := git.BranchExists(ctx, name)
		if err != nil {
			return err
		}
		if exists {
			if err := git.Checkout(ctx, name); err != nil {
				return err
			}
		} else {
			if err := git.CreateBranch(ctx, name); err != nil {
				return err
			}
		}
		task.Branch = name
		if err := db.UpdateTask(ctx, task); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Checked out task branch %s\n", name)
		return nil
	}

	if task.Branch == "" {
		task.Branch = branch
		return db.UpdateTask(ctx, task)
	}
	return nil
}
