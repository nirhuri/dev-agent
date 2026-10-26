package engine

import (
	"context"
	"time"
)

// Status describes engine progress.
type Status string

const (
	StatusIdle      Status = "idle"
	StatusRunning   Status = "running"
	StatusWaiting   Status = "waiting"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

// Capability describes what the engine can do in this environment.
type Capability struct {
	Provider          string
	AutomationLevel   string // "unattended", "hybrid", "unavailable"
	SupportsPrintMode bool
	SupportsWorkspace bool
	SupportsCancel    bool
	SupportsResume    bool
	Notes             string
}

// TaskRequest is a coding task handed to an engine.
type TaskRequest struct {
	IssueKey       string
	Summary        string
	Description    string
	Plan           string
	Clarifications []string
	Workspace      string
	Model          string
	Autonomous     bool
	ExtraPrompt    string
}

// Result is the outcome of an engine invocation.
type Result struct {
	Status     Status
	SessionID  string
	Output     string
	Summary    string
	Error      string
	StartedAt  time.Time
	FinishedAt time.Time
}

// ExecutionStatus is a live or last-known status snapshot.
type ExecutionStatus struct {
	Status    Status
	SessionID string
	Message   string
}

// Engine is a replaceable coding-engine adapter.
type Engine interface {
	Name() string
	CheckAvailability(ctx context.Context) (Capability, error)
	ImplementTask(ctx context.Context, req TaskRequest) (Result, error)
	ContinueTask(ctx context.Context, sessionID string, userInput string, workspace string) (Result, error)
	GetExecutionStatus(ctx context.Context) ExecutionStatus
	CancelExecution(ctx context.Context) error
}
