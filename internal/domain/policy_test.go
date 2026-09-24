package domain_test

import (
	"testing"
	"time"

	"github.com/Emmanuelzyronis/forge/internal/domain"
)

func TestRetryPolicy_ShouldRetry(t *testing.T) {
	p := domain.RetryPolicy{MaxAttempts: 3}

	cases := []struct {
		attemptCount int
		want         bool
	}{
		{0, true},
		{1, true},
		{2, true},
		{3, false}, // exhausted
		{4, false},
	}
	for _, tc := range cases {
		if got := p.ShouldRetry(tc.attemptCount); got != tc.want {
			t.Errorf("ShouldRetry(%d) = %v, want %v", tc.attemptCount, got, tc.want)
		}
	}
}

func TestRetryPolicy_DefaultPolicy(t *testing.T) {
	p := domain.DefaultRetryPolicy()
	if p.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", p.MaxAttempts)
	}
	if p.BackoffSecs != 0 {
		t.Errorf("BackoffSecs = %d, want 0", p.BackoffSecs)
	}
	if p.BackoffMultiplier != 1.0 {
		t.Errorf("BackoffMultiplier = %f, want 1.0", p.BackoffMultiplier)
	}
}

func TestRetryPolicy_NextEligibleAt_ZeroBackoff(t *testing.T) {
	p := domain.RetryPolicy{MaxAttempts: 3, BackoffSecs: 0, BackoffMultiplier: 1.0}
	now := time.Now().UTC()

	// With zero backoff, always returns now regardless of attempt number.
	for _, attempt := range []int{1, 2, 3} {
		got := p.NextEligibleAt(attempt, now)
		if !got.Equal(now) {
			t.Errorf("attempt %d: expected now (%v), got %v", attempt, now, got)
		}
	}
}

func TestRetryPolicy_NextEligibleAt_LinearBackoff(t *testing.T) {
	// Multiplier=1.0 → linear: delay = BackoffSecs * 1^(n-1) = BackoffSecs always.
	p := domain.RetryPolicy{MaxAttempts: 5, BackoffSecs: 10, BackoffMultiplier: 1.0}
	now := time.Now().UTC()

	for _, attempt := range []int{1, 2, 3} {
		got := p.NextEligibleAt(attempt, now)
		want := now.Add(10 * time.Second)
		if !got.Equal(want) {
			t.Errorf("attempt %d: expected %v, got %v", attempt, want, got)
		}
	}
}

func TestRetryPolicy_NextEligibleAt_ExponentialBackoff(t *testing.T) {
	// BackoffSecs=10, Multiplier=2.0 → delays: 10, 20, 40 seconds for attempts 1,2,3.
	p := domain.RetryPolicy{MaxAttempts: 5, BackoffSecs: 10, BackoffMultiplier: 2.0}
	now := time.Now().UTC()

	cases := []struct {
		attempt  int
		wantSecs float64
	}{
		{1, 10},  // 10 * 2^0 = 10
		{2, 20},  // 10 * 2^1 = 20
		{3, 40},  // 10 * 2^2 = 40
	}
	for _, tc := range cases {
		got := p.NextEligibleAt(tc.attempt, now)
		want := now.Add(time.Duration(tc.wantSecs * float64(time.Second)))
		if !got.Equal(want) {
			t.Errorf("attempt %d: expected %v, got %v", tc.attempt, want, got)
		}
	}
}

func TestRetryPolicy_NextEligibleAt_ZeroAttempt(t *testing.T) {
	p := domain.RetryPolicy{MaxAttempts: 3, BackoffSecs: 30, BackoffMultiplier: 2.0}
	now := time.Now().UTC()

	// Attempt <= 0 should return now (guard against nonsensical input).
	got := p.NextEligibleAt(0, now)
	if !got.Equal(now) {
		t.Errorf("attempt 0: expected now, got %v", got)
	}
}
