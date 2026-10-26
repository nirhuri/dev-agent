package pr

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/nirhuri/dev-agent/internal/workflow"
)

// Provider creates pull requests without merging them.
type Provider interface {
	Name() string
	Check(ctx context.Context) error
	Create(ctx context.Context, req CreateRequest) (CreateResult, error)
}

// CreateRequest describes a PR to open.
type CreateRequest struct {
	RepoDir    string
	HeadBranch string
	BaseBranch string
	Title      string
	Body       string
	Draft      bool
}

// CreateResult is returned after a verified create.
type CreateResult struct {
	URL string
}

// Runner executes external commands (injectable for tests).
type Runner func(ctx context.Context, dir, name string, args ...string) (string, error)

// GitHub uses the `gh` CLI. It never merges.
type GitHub struct {
	GHBin  string
	Runner Runner
}

func NewGitHub() *GitHub {
	return &GitHub{GHBin: "gh"}
}

func (g *GitHub) Name() string { return "github" }

func (g *GitHub) Check(ctx context.Context) error {
	bin := g.bin()
	if _, err := exec.LookPath(bin); err != nil {
		return fmt.Errorf("gh CLI not found; install from https://cli.github.com/")
	}
	out, err := g.run(ctx, "", bin, "auth", "status")
	if err != nil {
		return fmt.Errorf("gh not authenticated; run `gh auth login` (%v)", err)
	}
	_ = out
	return nil
}

func (g *GitHub) Create(ctx context.Context, req CreateRequest) (CreateResult, error) {
	if err := g.Check(ctx); err != nil {
		return CreateResult{}, err
	}
	if req.HeadBranch == "" {
		return CreateResult{}, fmt.Errorf("head branch is required")
	}
	if req.Title == "" {
		return CreateResult{}, fmt.Errorf("title is required")
	}

	args := []string{"pr", "create", "--title", req.Title, "--body", req.Body, "--head", req.HeadBranch}
	if req.BaseBranch != "" {
		args = append(args, "--base", req.BaseBranch)
	}
	if req.Draft {
		args = append(args, "--draft")
	}

	out, err := g.run(ctx, req.RepoDir, g.bin(), args...)
	if err != nil {
		return CreateResult{}, fmt.Errorf("gh pr create failed: %w", err)
	}
	url := extractURL(out)
	if url == "" {
		return CreateResult{}, fmt.Errorf("gh pr create succeeded but no PR URL found in output: %s", truncate(out, 200))
	}
	return CreateResult{URL: url}, nil
}

func (g *GitHub) bin() string {
	if g.GHBin == "" {
		return "gh"
	}
	return g.GHBin
}

func (g *GitHub) run(ctx context.Context, dir, name string, args ...string) (string, error) {
	if g.Runner != nil {
		return g.Runner(ctx, dir, name, args...)
	}
	cctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(cctx, name, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return stdout.String(), fmt.Errorf("%s", msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func extractURL(out string) string {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "https://github.com/") && strings.Contains(line, "/pull/") {
			return line
		}
	}
	// Sometimes gh prints only the URL.
	out = strings.TrimSpace(out)
	if strings.HasPrefix(out, "https://") && strings.Contains(out, "/pull/") {
		return strings.Fields(out)[0]
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// BuildBody drafts a PR description from task state.
func BuildBody(task *workflow.Task) string {
	var b strings.Builder
	b.WriteString("## Summary\n")
	if task.IssueSummary != "" {
		fmt.Fprintf(&b, "- %s (%s)\n", task.IssueSummary, task.IssueKey)
	} else {
		fmt.Fprintf(&b, "- Work for %s\n", task.IssueKey)
	}
	if task.Plan != "" {
		b.WriteString("\n## Plan\n")
		b.WriteString(task.Plan)
		b.WriteString("\n")
	}
	if len(task.Validation) > 0 {
		b.WriteString("\n## Tests\n")
		for _, v := range task.Validation {
			status := "pass"
			if !v.OK {
				status = "fail"
			}
			fmt.Fprintf(&b, "- [%s] `%s`\n", status, v.Command)
		}
	}
	if len(task.CommitHashes) > 0 {
		b.WriteString("\n## Commits\n")
		for _, h := range task.CommitHashes {
			fmt.Fprintf(&b, "- `%s`\n", h)
		}
	}
	b.WriteString("\n## Notes\n")
	b.WriteString("- Created by dev-agent\n")
	b.WriteString("- Do not merge automatically\n")
	return b.String()
}
