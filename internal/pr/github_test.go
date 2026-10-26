package pr_test

import (
	"context"
	"strings"
	"testing"

	"github.com/nirhuri/dev-agent/internal/pr"
	"github.com/nirhuri/dev-agent/internal/workflow"
)

func TestCreateParsesURL(t *testing.T) {
	g := pr.NewGitHub()
	g.Runner = func(ctx context.Context, dir, name string, args ...string) (string, error) {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "auth status") {
			return "ok", nil
		}
		if strings.Contains(joined, "pr create") {
			if !strings.Contains(joined, "--title") || !strings.Contains(joined, "--body") {
				t.Fatalf("missing title/body: %v", args)
			}
			if strings.Contains(joined, "merge") {
				t.Fatal("must never merge")
			}
			return "https://github.com/nirhuri/dev-agent/pull/42\n", nil
		}
		return "", nil
	}
	// Bypass LookPath by stubbing Check path — Runner handles auth status.
	// LookPath still runs in Check; if gh missing, skip.
	res, err := g.Create(context.Background(), pr.CreateRequest{
		RepoDir:    "/tmp/repo",
		HeadBranch: "agent/APP-1-x",
		Title:      "APP-1: test",
		Body:       "body",
	})
	if err != nil {
		// Environments without gh on PATH fail Check via LookPath.
		if strings.Contains(err.Error(), "gh CLI not found") {
			t.Skip(err.Error())
		}
		t.Fatal(err)
	}
	if res.URL != "https://github.com/nirhuri/dev-agent/pull/42" {
		t.Fatalf("url = %q", res.URL)
	}
}

func TestBuildBody(t *testing.T) {
	body := pr.BuildBody(&workflow.Task{
		IssueKey:     "APP-1",
		IssueSummary: "Do thing",
		Plan:         "edit files",
		Validation:   []workflow.ValidationResult{{Command: "go test ./...", OK: true}},
		CommitHashes: []string{"abc123"},
	})
	for _, want := range []string{"APP-1", "go test", "abc123", "Do not merge"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
}

func TestExtractURLRequiresPullPath(t *testing.T) {
	g := pr.NewGitHub()
	g.GHBin = "true" // exists on unix; auth will "succeed" via runner
	g.Runner = func(ctx context.Context, dir, name string, args ...string) (string, error) {
		if len(args) > 0 && args[0] == "auth" {
			return "ok", nil
		}
		return "created but no link", nil
	}
	_, err := g.Create(context.Background(), pr.CreateRequest{
		HeadBranch: "b", Title: "t", Body: "b",
	})
	if err == nil || !strings.Contains(err.Error(), "no PR URL") {
		t.Fatalf("err = %v", err)
	}
}
