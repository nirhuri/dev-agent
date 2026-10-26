package store_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/nirhuri/dev-agent/internal/store"
	"github.com/nirhuri/dev-agent/internal/workflow"
)

func TestCreateGetTransitionResume(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.db")
	s, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	task := &workflow.Task{
		IssueKey:     "APP-1",
		IssueSummary: "Test",
		Repository:   "/tmp/repo",
		Engine:       "cursor",
		State:        workflow.StateIdle,
	}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	if task.ID == "" {
		t.Fatal("expected id")
	}

	got, err := s.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.IssueKey != "APP-1" {
		t.Fatalf("got %+v", got)
	}

	updated, err := s.Transition(ctx, task.ID, workflow.StateIssueSelected, "selected")
	if err != nil {
		t.Fatal(err)
	}
	if updated.State != workflow.StateIssueSelected {
		t.Fatalf("state = %s", updated.State)
	}

	_, err = s.Transition(ctx, task.ID, workflow.StateCompleted, "bad")
	if err == nil {
		t.Fatal("expected invalid transition")
	}

	updated, err = s.Transition(ctx, task.ID, workflow.StateAnalyzing, "analyze")
	if err != nil {
		t.Fatal(err)
	}
	updated, err = s.Transition(ctx, task.ID, workflow.StateWaitingClarification, "need info")
	if err != nil {
		t.Fatal(err)
	}
	updated.Clarifications = []workflow.ClarificationQA{{Question: "Which auth?", Answer: ""}}
	if err := s.UpdateTask(ctx, updated); err != nil {
		t.Fatal(err)
	}

	byIssue, err := s.GetTaskByIssue(ctx, "APP-1")
	if err != nil {
		t.Fatal(err)
	}
	if byIssue.State != workflow.StateWaitingClarification {
		t.Fatalf("state = %s", byIssue.State)
	}

	events, err := s.ListTransitions(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) < 3 {
		t.Fatalf("events = %d", len(events))
	}
}

func TestInvalidTransitionRejected(t *testing.T) {
	s, err := store.OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	task := &workflow.Task{IssueKey: "X-1", State: workflow.StateCompleted}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	_, err = s.Transition(ctx, task.ID, workflow.StateAnalyzing, "nope")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLockPreventsConcurrentOwners(t *testing.T) {
	s, err := store.OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	task := &workflow.Task{IssueKey: "X-2", State: workflow.StateIdle}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err := s.AcquireLock(ctx, task.ID, "a", time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := s.AcquireLock(ctx, task.ID, "b", time.Minute); err == nil {
		t.Fatal("expected conflict")
	}
	if err := s.ReleaseLock(ctx, task.ID, "a"); err != nil {
		t.Fatal(err)
	}
	if err := s.AcquireLock(ctx, task.ID, "b", time.Minute); err != nil {
		t.Fatal(err)
	}
}

func TestCanTransitionMatrix(t *testing.T) {
	if !workflow.CanTransition(workflow.StateIdle, workflow.StateIssueSelected) {
		t.Fatal("idle -> issue_selected")
	}
	if workflow.CanTransition(workflow.StateIdle, workflow.StatePRCreated) {
		t.Fatal("idle -> pr_created should be false")
	}
	if !workflow.StateWaitingClarification.Resumable() {
		t.Fatal("waiting should be resumable")
	}
	if !workflow.StateCompleted.Terminal() {
		t.Fatal("completed should be terminal")
	}
}
