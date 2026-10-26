package e2e_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nirhuri/dev-agent/internal/engine"
	"github.com/nirhuri/dev-agent/internal/pr"
	"github.com/nirhuri/dev-agent/internal/repository"
	"github.com/nirhuri/dev-agent/internal/store"
	"github.com/nirhuri/dev-agent/internal/workflow"
)

// TestHappyPath walks the persisted workflow with fake engine + fake gh.
func TestHappyPath(t *testing.T) {
	repo := initGoRepo(t)
	dbPath := filepath.Join(t.TempDir(), "tasks.db")
	db, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()

	svc := &workflow.Service{Store: db, Owner: "e2e"}
	task, err := svc.StartFromIssue(ctx, "APP-100", "Add hello helper", repo, "fake")
	if err != nil {
		t.Fatal(err)
	}

	mustTransition(t, db, ctx, task.ID, workflow.StateAnalyzing, "analyze")
	task, err = db.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	task.Plan = "Add hello.go and a unit test"
	if err := db.UpdateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	mustTransition(t, db, ctx, task.ID, workflow.StatePlanReady, "plan")

	// Branch off main before changes.
	git := repository.NewGit(repo)
	branch := repository.SanitizeBranchSlug("agent/", "APP-100", "Add hello helper")
	if err := git.CreateBranch(ctx, branch); err != nil {
		t.Fatal(err)
	}
	task.Branch = branch
	_ = db.UpdateTask(ctx, task)

	mustTransition(t, db, ctx, task.ID, workflow.StateImplementing, "implement")
	fake := &engine.Fake{}
	res, err := fake.ImplementTask(ctx, engine.TaskRequest{
		IssueKey:  "APP-100",
		Workspace: repo,
		Plan:      task.Plan,
	})
	if err != nil || res.Status != engine.StatusSucceeded {
		t.Fatalf("engine: %v %+v", err, res)
	}
	// Simulate engine writing files.
	if err := os.WriteFile(filepath.Join(repo, "hello.go"), []byte("package hello\n\nfunc Hello() string { return \"hi\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "hello_test.go"), []byte("package hello\n\nimport \"testing\"\n\nfunc TestHello(t *testing.T) {\n\tif Hello() != \"hi\" { t.Fatal(Hello()) }\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	mustTransition(t, db, ctx, task.ID, workflow.StateValidating, "validate")
	results, err := repository.RunValidation(ctx, repo, []string{"go test ./..."})
	if err != nil {
		t.Fatal(err)
	}
	if !repository.AllOK(results) {
		t.Fatalf("validation failed: %+v", results)
	}
	task.Validation = results

	files, err := git.DiffNameOnly(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := git.StageFiles(ctx, files); err != nil {
		t.Fatal(err)
	}
	hash, err := git.Commit(ctx, "feat: APP-100 add hello helper")
	if err != nil {
		t.Fatal(err)
	}
	task.CommitHashes = []string{hash}
	_ = db.UpdateTask(ctx, task)
	mustTransition(t, db, ctx, task.ID, workflow.StateReadyForPR, "ready")

	gh := pr.NewGitHub()
	gh.GHBin = "true"
	gh.Runner = func(ctx context.Context, dir, name string, args ...string) (string, error) {
		if len(args) > 0 && args[0] == "auth" {
			return "ok", nil
		}
		return "https://github.com/example/repo/pull/7", nil
	}
	created, err := gh.Create(ctx, pr.CreateRequest{
		RepoDir:    repo,
		HeadBranch: branch,
		Title:      "APP-100: Add hello helper",
		Body:       pr.BuildBody(task),
	})
	if err != nil {
		t.Fatal(err)
	}
	task.PRURL = created.URL
	_ = db.UpdateTask(ctx, task)
	mustTransition(t, db, ctx, task.ID, workflow.StatePRCreated, created.URL)
	mustTransition(t, db, ctx, task.ID, workflow.StateCompleted, "done")

	final, err := db.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.State != workflow.StateCompleted {
		t.Fatalf("state = %s", final.State)
	}
	if final.PRURL == "" || !strings.Contains(final.PRURL, "/pull/") {
		t.Fatalf("pr url = %q", final.PRURL)
	}
	if len(final.CommitHashes) != 1 {
		t.Fatalf("commits = %v", final.CommitHashes)
	}

	events, err := db.ListTransitions(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) < 6 {
		t.Fatalf("expected audit trail, got %d", len(events))
	}
}

func TestFailurePathDirtyProtectedBranch(t *testing.T) {
	repo := initGoRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "dirty.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	git := repository.NewGit(repo)
	dirty, err := git.IsDirty(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !dirty {
		t.Fatal("expected dirty")
	}
	branch, _ := git.CurrentBranch(context.Background())
	if !repository.IsProtectedBranch(branch, []string{"main", "master"}) {
		t.Fatalf("expected protected branch, got %s", branch)
	}
}

func mustTransition(t *testing.T, db *store.SQLite, ctx context.Context, id string, to workflow.State, note string) {
	t.Helper()
	if _, err := db.Transition(ctx, id, to, note); err != nil {
		t.Fatalf("transition to %s: %v", to, err)
	}
}

func initGoRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
	run("init", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/hello\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "go.mod")
	run("commit", "-m", "init")
	_ = time.Now()
	return dir
}
