package domain

import (
	"time"

	"github.com/google/uuid"
)

// AttemptState represents a position in the attempt lifecycle state machine.
type AttemptState string

const (
	AttemptStateCreated   AttemptState = "CREATED"
	AttemptStateRunning   AttemptState = "RUNNING"
	AttemptStateSucceeded AttemptState = "SUCCEEDED"
	AttemptStateFailed    AttemptState = "FAILED"
	AttemptStateTimedOut  AttemptState = "TIMED_OUT"
	// AttemptStateAbandoned means the worker disappeared without an explicit outcome.
	// Distinct from FAILED: the worker did not report failure — it simply stopped.
	AttemptStateAbandoned AttemptState = "ABANDONED"
)

var terminalAttemptStates = map[AttemptState]bool{
	AttemptStateSucceeded: true,
	AttemptStateFailed:    true,
	AttemptStateTimedOut:  true,
	AttemptStateAbandoned: true,
}

var legalAttemptTransitions = map[AttemptState]map[AttemptState]bool{
	AttemptStateCreated: {AttemptStateRunning: true},
	AttemptStateRunning: {
		AttemptStateSucceeded: true,
		AttemptStateFailed:    true,
		AttemptStateTimedOut:  true,
		AttemptStateAbandoned: true,
	},
}

// ExecutionAttempt records a single worker's attempt to execute a job.
type ExecutionAttempt struct {
	ID          uuid.UUID
	JobID       uuid.UUID
	WorkerID    uuid.UUID
	AttemptNum  int
	State       AttemptState
	StartedAt   *time.Time
	FinishedAt  *time.Time
	ErrorDetail *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Transition advances the attempt to the given state, enforcing the attempt state machine.
func (a *ExecutionAttempt) Transition(to AttemptState) error {
	if terminalAttemptStates[a.State] {
		return &IllegalTransitionError{
			FromState: string(a.State),
			ToState:   string(to),
			Reason:    "terminal state cannot transition",
		}
	}
	allowed, ok := legalAttemptTransitions[a.State]
	if !ok || !allowed[to] {
		return &IllegalTransitionError{
			FromState: string(a.State),
			ToState:   string(to),
		}
	}
	a.State = to
	a.UpdatedAt = time.Now().UTC()
	return nil
}

// IsTerminal reports whether the attempt has reached a terminal state.
func (a *ExecutionAttempt) IsTerminal() bool {
	return terminalAttemptStates[a.State]
}
