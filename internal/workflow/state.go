package workflow

import (
	"fmt"
	"time"
)

// State is a task lifecycle state.
type State string

const (
	StateIdle                 State = "IDLE"
	StateIssueSelected        State = "ISSUE_SELECTED"
	StateAnalyzing            State = "ANALYZING"
	StateWaitingClarification State = "WAITING_FOR_CLARIFICATION"
	StatePlanReady            State = "PLAN_READY"
	StateImplementing         State = "IMPLEMENTING"
	StateValidating           State = "VALIDATING"
	StateRepairing            State = "REPAIRING"
	StateReadyForPR           State = "READY_FOR_PR"
	StatePRCreated            State = "PR_CREATED"
	StateCompleted            State = "COMPLETED"
	StateFailed               State = "FAILED"
	StateCancelled            State = "CANCELLED"
	StateBlocked              State = "BLOCKED"
)

// Terminal reports whether the state ends the workflow.
func (s State) Terminal() bool {
	switch s {
	case StateCompleted, StateFailed, StateCancelled:
		return true
	default:
		return false
	}
}

// Resumable reports whether a task in this state can be resumed.
func (s State) Resumable() bool {
	switch s {
	case StateWaitingClarification, StateBlocked, StateFailed,
		StateIssueSelected, StateAnalyzing, StatePlanReady,
		StateImplementing, StateValidating, StateRepairing, StateReadyForPR:
		return true
	default:
		return false
	}
}

var allowedTransitions = map[State][]State{
	StateIdle:                 {StateIssueSelected, StateCancelled},
	StateIssueSelected:        {StateAnalyzing, StateCancelled, StateFailed},
	StateAnalyzing:            {StateWaitingClarification, StatePlanReady, StateBlocked, StateFailed, StateCancelled},
	StateWaitingClarification: {StateAnalyzing, StatePlanReady, StateCancelled, StateFailed},
	StatePlanReady:            {StateImplementing, StateWaitingClarification, StateCancelled, StateFailed},
	StateImplementing:         {StateValidating, StateWaitingClarification, StateBlocked, StateFailed, StateCancelled},
	StateValidating:           {StateRepairing, StateReadyForPR, StateFailed, StateCancelled},
	StateRepairing:            {StateValidating, StateReadyForPR, StateFailed, StateCancelled, StateBlocked},
	StateReadyForPR:           {StatePRCreated, StateFailed, StateCancelled, StateBlocked},
	StatePRCreated:            {StateCompleted, StateFailed},
	StateBlocked:              {StateAnalyzing, StateImplementing, StateValidating, StateReadyForPR, StateCancelled, StateFailed},
	StateFailed:               {StateAnalyzing, StateImplementing, StateValidating, StateReadyForPR, StateCancelled},
	StateCancelled:            {},
	StateCompleted:            {},
}

// CanTransition reports whether from -> to is allowed.
func CanTransition(from, to State) bool {
	for _, next := range allowedTransitions[from] {
		if next == to {
			return true
		}
	}
	return false
}

// TransitionError describes an illegal state change.
type TransitionError struct {
	From State
	To   State
}

func (e *TransitionError) Error() string {
	return fmt.Sprintf("invalid transition %s -> %s", e.From, e.To)
}

// ClarificationQA is a persisted clarification question and optional answer.
type ClarificationQA struct {
	Question string `json:"question"`
	Answer   string `json:"answer,omitempty"`
}

// ValidationResult records a validation run.
type ValidationResult struct {
	Command  string `json:"command"`
	OK       bool   `json:"ok"`
	Output   string `json:"output,omitempty"`
	ExitCode int    `json:"exit_code"`
}

// Task is a persisted workflow task.
type Task struct {
	ID             string             `json:"id"`
	IssueKey       string             `json:"issue_key"`
	IssueSummary   string             `json:"issue_summary"`
	Repository     string             `json:"repository"`
	Branch         string             `json:"branch"`
	Engine         string             `json:"engine"`
	State          State              `json:"state"`
	Plan           string             `json:"plan"`
	Clarifications []ClarificationQA  `json:"clarifications"`
	Validation     []ValidationResult `json:"validation"`
	CommitHashes   []string           `json:"commit_hashes"`
	PRURL          string             `json:"pr_url"`
	ErrorMessage   string             `json:"error_message"`
	RepairAttempt  int                `json:"repair_attempt"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
}

// TransitionEvent is an audit record of a state change.
type TransitionEvent struct {
	ID        int64
	TaskID    string
	FromState State
	ToState   State
	Note      string
	CreatedAt time.Time
}
