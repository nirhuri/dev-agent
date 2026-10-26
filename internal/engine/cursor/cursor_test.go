package cursor_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nirhuri/dev-agent/internal/engine"
	"github.com/nirhuri/dev-agent/internal/engine/cursor"
)

func TestImplementTaskBuildsArgs(t *testing.T) {
	var sawName string
	var sawArgs []string
	var sawEnv []string

	a := cursor.New("agent", "test-key", "gpt-test")
	a.Runner = func(ctx context.Context, name string, args []string, env []string) (string, string, error) {
		sawName = name
		sawArgs = append([]string{}, args...)
		sawEnv = append([]string{}, env...)
		return "done: edited main.go", "", nil
	}

	res, err := a.ImplementTask(context.Background(), engine.TaskRequest{
		IssueKey:    "APP-1",
		Summary:     "Do thing",
		Description: "Details",
		Plan:        "1. edit",
		Workspace:   "/tmp/repo",
		Autonomous:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != engine.StatusSucceeded {
		t.Fatalf("status = %s", res.Status)
	}
	if sawName == "" || (!strings.Contains(sawName, "agent") && sawName != "agent") {
		t.Fatalf("name = %q", sawName)
	}
	joined := strings.Join(sawArgs, " ")
	for _, want := range []string{"--print", "--workspace", "/tmp/repo", "--force", "--model", "gpt-test", "APP-1"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args missing %q: %v", want, sawArgs)
		}
	}
	foundKey := false
	for _, e := range sawEnv {
		if e == "CURSOR_API_KEY=test-key" {
			foundKey = true
		}
	}
	if !foundKey {
		t.Fatalf("env missing API key: %v", sawEnv)
	}
}

func TestImplementTaskFailure(t *testing.T) {
	a := cursor.New("agent", "", "")
	a.Runner = func(ctx context.Context, name string, args []string, env []string) (string, string, error) {
		return "", "boom", errors.New("exit 1")
	}
	res, err := a.ImplementTask(context.Background(), engine.TaskRequest{Workspace: "/tmp"})
	if err == nil {
		t.Fatal("expected error")
	}
	if res.Status != engine.StatusFailed {
		t.Fatalf("status = %s", res.Status)
	}
}

func TestCancelDuringRun(t *testing.T) {
	a := cursor.New("agent", "", "")
	started := make(chan struct{})
	a.Runner = func(ctx context.Context, name string, args []string, env []string) (string, string, error) {
		close(started)
		<-ctx.Done()
		return "", "", ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = a.ImplementTask(ctx, engine.TaskRequest{})
	}()
	<-started
	_ = a.CancelExecution(context.Background())
	cancel()
	<-done
	st := a.GetExecutionStatus(context.Background())
	if st.Status != engine.StatusCancelled {
		t.Fatalf("status = %s", st.Status)
	}
}

func TestCheckAvailabilityMissingBinary(t *testing.T) {
	a := cursor.New("definitely-not-a-real-agent-binary-xyz", "", "")
	cap, err := a.CheckAvailability(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if cap.AutomationLevel != "unavailable" {
		t.Fatalf("level = %s", cap.AutomationLevel)
	}
}
