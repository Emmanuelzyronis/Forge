package domain

import "time"

// RetryPolicy governs when and how many times a failed job is retried (F-INV-005).
type RetryPolicy struct {
	MaxAttempts  int
	InitialDelay time.Duration // default: 5s when zero
	MaxDelay     time.Duration // default: 30m when zero
}

// DefaultRetryPolicy returns the standard 3-attempt policy with 5s base and 30m cap.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts:  3,
		InitialDelay: 5 * time.Second,
		MaxDelay:     30 * time.Minute,
	}
}

// ShouldRetry returns true if the job should be retried given the current attempt count.
func (p RetryPolicy) ShouldRetry(attemptCount int) bool {
	return attemptCount < p.MaxAttempts
}

// NextEligibleAt returns the earliest time the job may be claimed again using
// exponential backoff: min(InitialDelay * 2^(attemptNumber-1), MaxDelay).
// attemptNumber is 1-based (the attempt that just failed).
// Zero values fall back to 5s base and 30m cap.
func (p RetryPolicy) NextEligibleAt(attemptNumber int, now time.Time) time.Time {
	base := p.InitialDelay
	if base <= 0 {
		base = 5 * time.Second
	}
	max := p.MaxDelay
	if max <= 0 {
		max = 30 * time.Minute
	}
	if attemptNumber < 1 {
		attemptNumber = 1
	}
	exp := attemptNumber - 1
	if exp > 30 { // cap to avoid int overflow on 2^exp
		exp = 30
	}
	delay := base * (1 << uint(exp))
	if delay > max || delay < 0 { // second guard for any overflow that slipped through
		delay = max
	}
	return now.Add(delay)
}
