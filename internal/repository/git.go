package repository

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Git runs Git CLI commands in a repository.
type Git struct {
	Dir    string
	GitBin string
	Runner func(ctx context.Context, dir, name string, args ...string) (string, error)
}

// NewGit creates a Git helper for dir.
func NewGit(dir string) *Git {
	return &Git{Dir: dir, GitBin: "git"}
}

func (g *Git) run(ctx context.Context, args ...string) (string, error) {
	bin := g.GitBin
	if bin == "" {
		bin = "git"
	}
	if g.Runner != nil {
		return g.Runner(ctx, g.Dir, bin, args...)
	}
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, bin, args...)
	cmd.Dir = g.Dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return stdout.String(), fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimRight(stdout.String(), "\n"), nil
}

// EnsureRepo verifies the directory is a git work tree.
func (g *Git) EnsureRepo(ctx context.Context) error {
	out, err := g.run(ctx, "rev-parse", "--is-inside-work-tree")
	if err != nil || out != "true" {
		return fmt.Errorf("%s is not a git repository", g.Dir)
	}
	return nil
}

// CurrentBranch returns the current branch name.
func (g *Git) CurrentBranch(ctx context.Context) (string, error) {
	return g.run(ctx, "rev-parse", "--abbrev-ref", "HEAD")
}

// StatusPorcelain returns porcelain status lines.
func (g *Git) StatusPorcelain(ctx context.Context) (string, error) {
	return g.run(ctx, "status", "--porcelain")
}

// IsDirty reports whether the working tree has uncommitted changes.
func (g *Git) IsDirty(ctx context.Context) (bool, error) {
	out, err := g.StatusPorcelain(ctx)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// IsProtectedBranch reports whether branch is protected.
func IsProtectedBranch(branch string, protected []string) bool {
	for _, p := range protected {
		if branch == p {
			return true
		}
	}
	return false
}

// CreateBranch creates and checks out a new branch from the current HEAD.
func (g *Git) CreateBranch(ctx context.Context, name string) error {
	_, err := g.run(ctx, "checkout", "-b", name)
	return err
}

// Checkout switches to an existing branch.
func (g *Git) Checkout(ctx context.Context, name string) error {
	_, err := g.run(ctx, "checkout", name)
	return err
}

// BranchExists reports whether a local branch exists.
func (g *Git) BranchExists(ctx context.Context, name string) (bool, error) {
	_, err := g.run(ctx, "show-ref", "--verify", "--quiet", "refs/heads/"+name)
	if err != nil {
		return false, nil
	}
	return true, nil
}

// DiffNameOnly lists changed files (including unstaged).
func (g *Git) DiffNameOnly(ctx context.Context) ([]string, error) {
	out, err := g.run(ctx, "diff", "--name-only", "HEAD")
	if err != nil {
		return nil, err
	}
	untracked, err := g.run(ctx, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	for _, line := range strings.Split(untracked, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return unique(files), nil
}

// StageFiles stages explicit paths only (never ".").
func (g *Git) StageFiles(ctx context.Context, files []string) error {
	if len(files) == 0 {
		return fmt.Errorf("no files to stage")
	}
	for _, f := range files {
		if f == "." || f == ".." || strings.TrimSpace(f) == "" {
			return fmt.Errorf("refusing to stage %q; pass explicit paths", f)
		}
		if strings.HasPrefix(f, "-") {
			return fmt.Errorf("refusing suspicious path %q", f)
		}
	}
	args := append([]string{"add", "--"}, files...)
	_, err := g.run(ctx, args...)
	return err
}

// Commit creates a commit from the index.
func (g *Git) Commit(ctx context.Context, message string) (string, error) {
	if strings.TrimSpace(message) == "" {
		return "", fmt.Errorf("commit message is required")
	}
	if _, err := g.run(ctx, "commit", "-m", message); err != nil {
		return "", err
	}
	return g.run(ctx, "rev-parse", "HEAD")
}

// StagedDiff returns the staged diff for review.
func (g *Git) StagedDiff(ctx context.Context) (string, error) {
	return g.run(ctx, "diff", "--cached")
}

// RemoteURL returns the URL for a remote.
func (g *Git) RemoteURL(ctx context.Context, name string) (string, error) {
	if name == "" {
		name = "origin"
	}
	return g.run(ctx, "remote", "get-url", name)
}

// Push pushes the named branch to origin without force.
func (g *Git) Push(ctx context.Context, branch string) error {
	_, err := g.run(ctx, "push", "-u", "origin", branch)
	return err
}

// SanitizeBranchSlug builds agent/<KEY>-slug.
func SanitizeBranchSlug(prefix, issueKey, summary string) string {
	if prefix == "" {
		prefix = "agent/"
	}
	slug := strings.ToLower(summary)
	repl := strings.NewReplacer(" ", "-", "/", "-", "_", "-", ".", "-")
	slug = repl.Replace(slug)
	var b strings.Builder
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	slug = strings.Trim(b.String(), "-")
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	if len(slug) > 40 {
		slug = slug[:40]
		slug = strings.Trim(slug, "-")
	}
	if slug == "" {
		slug = "task"
	}
	return prefix + strings.ToUpper(issueKey) + "-" + slug
}

func unique(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// Abs returns an absolute path for the repo dir.
func (g *Git) Abs() (string, error) {
	return filepath.Abs(g.Dir)
}
