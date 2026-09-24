package domain

import (
	"math"
	"time"
)

// RetryPolicy governs when and how many times a failed job is retried (F-INV-005).
type RetryPolicy struct {
	MaxAttempts        int
	BackoffSecs        int64
	BackoffMultiplier  float64
}

// DefaultRetryPolicy returns the standard 3-attempt, no-backoff policy.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts:       3,
		BackoffSecs:       0,
		BackoffMultiplier: 1.0,
	}
}

// ShouldRetry returns true if the job should be retried given the current attempt count.
func (p RetryPolicy) ShouldRetry(attemptCount int) bool {
	return attemptCount < p.MaxAttempts
}

// NextEligibleAt returns the earliest time the job may be claimed again.
// Exponential backoff: BackoffSecs * BackoffMultiplier^(attemptNumber-1).
// If BackoffSecs is 0 the job is immediately eligible.
func (p RetryPolicy) NextEligibleAt(attemptNumber int, now time.Time) time.Time {
	if p.BackoffSecs == 0 || attemptNumber <= 0 {
		return now
	}
	exp := math.Pow(p.BackoffMultiplier, float64(attemptNumber-1))
	delaySecs := float64(p.BackoffSecs) * exp
	return now.Add(time.Duration(delaySecs * float64(time.Second)))
}
