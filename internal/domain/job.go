package domain

import (
	"time"

	"github.com/google/uuid"
)

// JobState represents a position in the job lifecycle state machine.
type JobState string

const (
	JobStateQueued    JobState = "QUEUED"
	JobStateClaimed   JobState = "CLAIMED"
	JobStateRunning   JobState = "RUNNING"
	JobStateSucceeded JobState = "SUCCEEDED"
	JobStateFailed    JobState = "FAILED"
	JobStateTimedOut  JobState = "TIMED_OUT"
)

var terminalJobStates = map[JobState]bool{
	JobStateSucceeded: true,
	JobStateFailed:    true,
	JobStateTimedOut:  true,
}

// legalJobTransitions encodes the allowed edges in the job state machine (F-INV-001).
var legalJobTransitions = map[JobState]map[JobState]bool{
	JobStateQueued:  {JobStateClaimed: true},
	JobStateClaimed: {JobStateRunning: true, JobStateQueued: true}, // re-queued on lease expiry
	JobStateRunning: {JobStateSucceeded: true, JobStateFailed: true, JobStateTimedOut: true, JobStateQueued: true},
}

// Job is the root aggregate for a unit of work submitted to FORGE.
type Job struct {
	ID             uuid.UUID
	Kind           string
	Payload        []byte
	State          JobState
	Priority       int
	MaxAttempts    int
	AttemptCount   int
	LeaseToken     *uuid.UUID
	LeaseExpiresAt *time.Time
	EligibleAt     time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
	CorrelationID  *string
	IdempotencyKey *string
}

// Transition advances the job to the given state, enforcing F-INV-001 and F-INV-008.
func (j *Job) Transition(to JobState) error {
	// F-INV-008: terminal states cannot transition.
	if terminalJobStates[j.State] {
		return &IllegalTransitionError{
			FromState: string(j.State),
			ToState:   string(to),
			Reason:    "terminal state cannot transition",
		}
	}
	allowed, ok := legalJobTransitions[j.State]
	if !ok || !allowed[to] {
		return &IllegalTransitionError{
			FromState: string(j.State),
			ToState:   string(to),
		}
	}
	j.State = to
	j.UpdatedAt = time.Now().UTC()
	return nil
}

// IsTerminal reports whether the job is in a terminal state.
func (j *Job) IsTerminal() bool {
	return terminalJobStates[j.State]
}
