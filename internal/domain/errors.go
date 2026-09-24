package domain

import "fmt"

// IllegalTransitionError is returned when a state transition is not permitted by the state machine.
type IllegalTransitionError struct {
	FromState string
	ToState   string
	Reason    string
}

func (e *IllegalTransitionError) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("illegal transition %s → %s: %s", e.FromState, e.ToState, e.Reason)
	}
	return fmt.Sprintf("illegal transition %s → %s", e.FromState, e.ToState)
}

// StaleLeaseError is returned when a worker presents a lease token that no longer matches the record.
type StaleLeaseError struct {
	Reason string
}

func (e *StaleLeaseError) Error() string {
	return fmt.Sprintf("stale lease: %s", e.Reason)
}

// RetryExhaustedError is returned when a job has exhausted its maximum retry attempts.
type RetryExhaustedError struct {
	AttemptCount int
	MaxAttempts  int
}

func (e *RetryExhaustedError) Error() string {
	return fmt.Sprintf("retry exhausted: %d of %d attempts used", e.AttemptCount, e.MaxAttempts)
}
