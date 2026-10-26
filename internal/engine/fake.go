package engine

import (
	"context"
	"sync"
	"time"
)

// Fake is an in-memory engine for tests.
type Fake struct {
	mu       sync.Mutex
	Avail    Capability
	AvailErr error
	Results  []Result
	Calls    []TaskRequest
	Continue []string
	status   ExecutionStatus
}

func (f *Fake) Name() string { return "fake" }

func (f *Fake) CheckAvailability(ctx context.Context) (Capability, error) {
	if f.Avail.Provider == "" {
		f.Avail = Capability{Provider: "fake", AutomationLevel: "unattended", Notes: "test double"}
	}
	return f.Avail, f.AvailErr
}

func (f *Fake) ImplementTask(ctx context.Context, req TaskRequest) (Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, req)
	f.status = ExecutionStatus{Status: StatusRunning}
	res := Result{Status: StatusSucceeded, Summary: "fake ok", StartedAt: time.Now().UTC(), FinishedAt: time.Now().UTC()}
	if len(f.Results) > 0 {
		res = f.Results[0]
		f.Results = f.Results[1:]
	}
	f.status = ExecutionStatus{Status: res.Status, Message: res.Summary}
	return res, nil
}

func (f *Fake) ContinueTask(ctx context.Context, sessionID, userInput, workspace string) (Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Continue = append(f.Continue, userInput)
	return Result{Status: StatusSucceeded, SessionID: sessionID, Summary: "continued"}, nil
}

func (f *Fake) GetExecutionStatus(ctx context.Context) ExecutionStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status
}

func (f *Fake) CancelExecution(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status = ExecutionStatus{Status: StatusCancelled}
	return nil
}
