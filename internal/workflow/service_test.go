package workflow_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/nirhuri/dev-agent/internal/store"
	"github.com/nirhuri/dev-agent/internal/workflow"
)

func TestServiceStartAnswerResume(t *testing.T) {
	db, err := store.OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	svc := &workflow.Service{Store: db, Owner: "test"}

	task, err := svc.StartFromIssue(ctx, "app-10", "Summary", "/repo", "cursor")
	if err != nil {
		t.Fatal(err)
	}
	if task.IssueKey != "APP-10" || task.State != workflow.StateIssueSelected {
		t.Fatalf("task = %+v", task)
	}

	again, err := svc.StartFromIssue(ctx, "APP-10", "Summary", "/repo", "cursor")
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != task.ID {
		t.Fatal("expected existing task reused")
	}

	if _, err := db.Transition(ctx, task.ID, workflow.StateAnalyzing, ""); err != nil {
		t.Fatal(err)
	}
	waiting, err := svc.SetWaitingClarification(ctx, task.ID, []string{"Which library?"})
	if err != nil {
		t.Fatal(err)
	}
	if waiting.State != workflow.StateWaitingClarification {
		t.Fatalf("state = %s", waiting.State)
	}

	resumed, err := svc.Resume(ctx, "APP-10")
	if err != nil {
		t.Fatal(err)
	}
	if resumed.State != workflow.StateWaitingClarification {
		t.Fatal(resumed.State)
	}
	answered, err := svc.AnswerClarifications(ctx, resumed.ID, []string{"stdlib"})
	if err != nil {
		t.Fatal(err)
	}
	if answered.State != workflow.StateAnalyzing {
		t.Fatalf("state = %s", answered.State)
	}
	_ = svc.Release(ctx, resumed.ID)
}
