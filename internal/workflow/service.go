package workflow

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Service orchestrates task lifecycle operations on top of a Store.
type Service struct {
	Store Store
	Owner string
}

// StartFromIssue creates a task for an issue, or returns an existing active one.
func (s *Service) StartFromIssue(ctx context.Context, issueKey, summary, repo, engine string) (*Task, error) {
	issueKey = strings.ToUpper(strings.TrimSpace(issueKey))
	if issueKey == "" {
		return nil, fmt.Errorf("issue key is required")
	}
	existing, err := s.Store.GetTaskByIssue(ctx, issueKey)
	if err == nil {
		return existing, nil
	}
	if err != ErrNotFound {
		return nil, err
	}
	task := &Task{
		IssueKey:     issueKey,
		IssueSummary: summary,
		Repository:   repo,
		Engine:       engine,
		State:        StateIdle,
	}
	if err := s.Store.CreateTask(ctx, task); err != nil {
		return nil, err
	}
	return s.Store.Transition(ctx, task.ID, StateIssueSelected, "issue selected")
}

// AnswerClarifications records answers and moves the task back to analyzing.
func (s *Service) AnswerClarifications(ctx context.Context, taskID string, answers []string) (*Task, error) {
	task, err := s.Store.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task.State != StateWaitingClarification {
		return nil, fmt.Errorf("task is %s, not waiting for clarification", task.State)
	}
	pending := 0
	ai := 0
	for i := range task.Clarifications {
		if task.Clarifications[i].Answer == "" {
			pending++
			if ai < len(answers) {
				task.Clarifications[i].Answer = answers[ai]
				ai++
			}
		}
	}
	if ai < len(answers) {
		return nil, fmt.Errorf("too many answers: %d pending questions", pending)
	}
	for _, qa := range task.Clarifications {
		if strings.TrimSpace(qa.Answer) == "" {
			return nil, fmt.Errorf("unanswered question: %s", qa.Question)
		}
	}
	if err := s.Store.UpdateTask(ctx, task); err != nil {
		return nil, err
	}
	return s.Store.Transition(ctx, task.ID, StateAnalyzing, "clarifications answered")
}

// SetWaitingClarification persists questions and parks the task.
func (s *Service) SetWaitingClarification(ctx context.Context, taskID string, questions []string) (*Task, error) {
	task, err := s.Store.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	for _, q := range questions {
		q = strings.TrimSpace(q)
		if q == "" {
			continue
		}
		task.Clarifications = append(task.Clarifications, ClarificationQA{Question: q})
	}
	if err := s.Store.UpdateTask(ctx, task); err != nil {
		return nil, err
	}
	if task.State == StateWaitingClarification {
		return task, nil
	}
	return s.Store.Transition(ctx, task.ID, StateWaitingClarification, "awaiting user input")
}

// Resume loads a task by ID or issue key and validates it is resumable.
func (s *Service) Resume(ctx context.Context, idOrKey string) (*Task, error) {
	task, err := s.resolve(ctx, idOrKey)
	if err != nil {
		return nil, err
	}
	if !task.State.Resumable() {
		return nil, fmt.Errorf("task %s is in state %s and cannot be resumed", task.IssueKey, task.State)
	}
	owner := s.Owner
	if owner == "" {
		owner = "cli"
	}
	if err := s.Store.AcquireLock(ctx, task.ID, owner, 30*time.Minute); err != nil {
		return nil, err
	}
	return task, nil
}

// Status lists active (or all) tasks.
func (s *Service) Status(ctx context.Context, all bool) ([]Task, error) {
	return s.Store.ListTasks(ctx, all)
}

func (s *Service) resolve(ctx context.Context, idOrKey string) (*Task, error) {
	idOrKey = strings.TrimSpace(idOrKey)
	task, err := s.Store.GetTask(ctx, idOrKey)
	if err == nil {
		return task, nil
	}
	if err != ErrNotFound {
		return nil, err
	}
	return s.Store.GetTaskByIssue(ctx, strings.ToUpper(idOrKey))
}

// Release unlocks a task after resume work finishes.
func (s *Service) Release(ctx context.Context, taskID string) error {
	owner := s.Owner
	if owner == "" {
		owner = "cli"
	}
	err := s.Store.ReleaseLock(ctx, taskID, owner)
	if err == ErrConflict {
		return nil
	}
	return err
}
