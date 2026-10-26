package workflow

import (
	"context"
	"errors"
	"time"
)

// Store persists tasks and transition history.
type Store interface {
	CreateTask(ctx context.Context, task *Task) error
	GetTask(ctx context.Context, id string) (*Task, error)
	GetTaskByIssue(ctx context.Context, issueKey string) (*Task, error)
	ListTasks(ctx context.Context, includeTerminal bool) ([]Task, error)
	UpdateTask(ctx context.Context, task *Task) error
	Transition(ctx context.Context, taskID string, to State, note string) (*Task, error)
	ListTransitions(ctx context.Context, taskID string) ([]TransitionEvent, error)
	AcquireLock(ctx context.Context, taskID, owner string, ttl time.Duration) error
	ReleaseLock(ctx context.Context, taskID, owner string) error
	Close() error
}

var (
	// ErrNotFound indicates a missing task.
	ErrNotFound = errors.New("task not found")
	// ErrConflict indicates a concurrent modification or lock conflict.
	ErrConflict = errors.New("conflict")
	// ErrInvalidTransition wraps an illegal state change.
	ErrInvalidTransition = errors.New("invalid transition")
)
