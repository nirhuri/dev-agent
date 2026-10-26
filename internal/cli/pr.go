package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nirhuri/dev-agent/internal/config"
	"github.com/nirhuri/dev-agent/internal/pr"
	"github.com/nirhuri/dev-agent/internal/repository"
	"github.com/nirhuri/dev-agent/internal/store"
	"github.com/nirhuri/dev-agent/internal/workflow"
)

func runCreatePR(cmd *cobra.Command, cfg config.Config, db *store.SQLite, task *workflow.Task) error {
	if cfg.Git.PRProvider != "" && cfg.Git.PRProvider != "github" {
		return fmt.Errorf("unsupported pr provider %q", cfg.Git.PRProvider)
	}
	if task.Branch == "" {
		return fmt.Errorf("task has no branch; cannot create PR")
	}
	if task.Repository == "" {
		return fmt.Errorf("task has no repository path")
	}

	git := repository.NewGit(task.Repository)
	ctx := cmd.Context()
	if err := git.EnsureRepo(ctx); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Pushing branch %s...\n", task.Branch)
	if err := git.Push(ctx, task.Branch); err != nil {
		return fmt.Errorf("push failed (branch/commits preserved): %w\nNext: push manually then re-run resume", err)
	}

	provider := pr.NewGitHub()
	if err := provider.Check(ctx); err != nil {
		return fmt.Errorf("%w\nBranch was pushed; create the PR with `gh pr create` or fix auth and resume", err)
	}

	base := "main"
	for _, b := range cfg.Git.ProtectedBranches {
		if b != "" {
			base = b
			break
		}
	}

	title := fmt.Sprintf("%s: %s", task.IssueKey, shortSummary(task.IssueSummary))
	body := pr.BuildBody(task)
	fmt.Fprintln(cmd.OutOrStdout(), "Creating pull request (will not merge)...")
	result, err := provider.Create(ctx, pr.CreateRequest{
		RepoDir:    task.Repository,
		HeadBranch: task.Branch,
		BaseBranch: base,
		Title:      title,
		Body:       body,
	})
	if err != nil {
		return fmt.Errorf("PR create failed (branch preserved): %w", err)
	}

	task.PRURL = result.URL
	if err := db.UpdateTask(ctx, task); err != nil {
		return err
	}
	if _, err := db.Transition(ctx, task.ID, workflow.StatePRCreated, result.URL); err != nil {
		return err
	}
	if _, err := db.Transition(ctx, task.ID, workflow.StateCompleted, "done"); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Pull request: %s\n", result.URL)
	fmt.Fprintln(cmd.OutOrStdout(), "PR was NOT merged. Review and merge manually when ready.")
	return nil
}
