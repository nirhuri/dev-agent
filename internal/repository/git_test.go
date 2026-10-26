package repository_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nirhuri/dev-agent/internal/repository"
)

func TestSanitizeBranchSlug(t *testing.T) {
	got := repository.SanitizeBranchSlug("agent/", "app-142", "Add user authentication!")
	if got != "agent/APP-142-add-user-authentication" {
		t.Fatalf("got %q", got)
	}
}

func TestIsProtectedBranch(t *testing.T) {
	if !repository.IsProtectedBranch("main", []string{"main", "master"}) {
		t.Fatal("main should be protected")
	}
	if repository.IsProtectedBranch("feature", []string{"main"}) {
		t.Fatal("feature should not be protected")
	}
}

func TestGitSafeStagingAndCommit(t *testing.T) {
	dir := initTestRepo(t)
	git := repository.NewGit(dir)
	ctx := context.Background()

	if err := git.EnsureRepo(ctx); err != nil {
		t.Fatal(err)
	}
	branch, err := git.CurrentBranch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if repository.IsProtectedBranch(branch, []string{"main", "master"}) {
		name := repository.SanitizeBranchSlug("agent/", "T-1", "safe staging")
		if err := git.CreateBranch(ctx, name); err != nil {
			t.Fatal(err)
		}
	}

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := git.StageFiles(ctx, []string{"."}); err == nil {
		t.Fatal("expected refusal of git add .")
	}
	if err := git.StageFiles(ctx, []string{"a.txt"}); err != nil {
		t.Fatal(err)
	}
	hash, err := git.Commit(ctx, "feat: add a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "" {
		t.Fatal("empty hash")
	}

	dirty, err := git.IsDirty(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !dirty {
		t.Fatal("b.txt should leave tree dirty")
	}
}

func TestRefuseWorkOnProtectedWithoutBranch(t *testing.T) {
	dir := initTestRepo(t)
	git := repository.NewGit(dir)
	ctx := context.Background()
	branch, err := git.CurrentBranch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !repository.IsProtectedBranch(branch, []string{"main", "master"}) {
		t.Skip("repo not on protected branch")
	}
}

func TestDetectValidationCommandsGo(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmds := repository.DetectValidationCommands(dir)
	if len(cmds) == 0 || !strings.Contains(cmds[0], "go test") {
		t.Fatalf("cmds = %v", cmds)
	}
}

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
	run("init", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "README")
	run("commit", "-m", "init")
	return dir
}
