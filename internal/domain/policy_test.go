package domain_test

import (
	"testing"
	"time"

	"github.com/Emmanuelzyronis/forge/internal/domain"
)

func TestRetryPolicy_ShouldRetry(t *testing.T) {
	p := domain.RetryPolicy{MaxAttempts: 3}
	if !p.ShouldRetry(0) {
		t.Error("attempt 0 of 3 should retry")
	}
	if !p.ShouldRetry(1) {
		t.Error("attempt 1 of 3 should retry")
	}
	if !p.ShouldRetry(2) {
		t.Error("attempt 2 of 3 should retry")
	}
	if p.ShouldRetry(3) {
		t.Error("attempt 3 of 3 should not retry")
	}
	if p.ShouldRetry(4) {
		t.Error("attempt 4 of 3 should not retry")
	}
}

func TestRetryPolicy_DefaultPolicy(t *testing.T) {
	p := domain.DefaultRetryPolicy()
	if p.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", p.MaxAttempts)
	}
	if p.InitialDelay != 5*time.Second {
		t.Errorf("InitialDelay = %v, want 5s", p.InitialDelay)
	}
	if p.MaxDelay != 30*time.Minute {
		t.Errorf("MaxDelay = %v, want 30m", p.MaxDelay)
	}
}

func TestRetryPolicy_NextEligibleAt_ExponentialBackoff(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	p := domain.RetryPolicy{
		MaxAttempts:  5,
		InitialDelay: 10 * time.Second,
		MaxDelay:     5 * time.Minute,
	}

	tests := []struct {
		attemptNum int
		wantDelay  time.Duration
	}{
		{1, 10 * time.Second},        // 10s * 2^0 = 10s
		{2, 20 * time.Second},        // 10s * 2^1 = 20s
		{3, 40 * time.Second},        // 10s * 2^2 = 40s
		{4, 80 * time.Second},        // 10s * 2^3 = 80s
		{5, 160 * time.Second},       // 10s * 2^4 = 160s (under 5m cap)
		{6, 5 * time.Minute},         // 10s * 2^5 = 320s → capped at 5m
	}
	for _, tt := range tests {
		got := p.NextEligibleAt(tt.attemptNum, now)
		if got.Sub(now) != tt.wantDelay {
			t.Errorf("attempt %d: want delay %v, got %v", tt.attemptNum, tt.wantDelay, got.Sub(now))
		}
	}
}

func TestRetryPolicy_NextEligibleAt_Defaults(t *testing.T) {
	// Zero InitialDelay/MaxDelay → defaults to 5s base, 30m cap.
	p := domain.RetryPolicy{MaxAttempts: 3}
	now := time.Now()
	got := p.NextEligibleAt(1, now)
	want := 5 * time.Second
	if got.Sub(now) != want {
		t.Errorf("default attempt 1: want %v delay, got %v", want, got.Sub(now))
	}
}

func TestRetryPolicy_NextEligibleAt_OverflowSafe(t *testing.T) {
	// Large attempt number must not panic or produce a past/over-max eligible_at.
	p := domain.RetryPolicy{MaxAttempts: 100, InitialDelay: time.Second, MaxDelay: time.Hour}
	now := time.Now()
	got := p.NextEligibleAt(100, now)
	if got.Before(now) {
		t.Error("large attempt number produced past eligible_at")
	}
	if got.Sub(now) > time.Hour {
		t.Errorf("large attempt number exceeded MaxDelay: got %v", got.Sub(now))
	}
}

func TestRetryPolicy_NextEligibleAt_ZeroAttempt(t *testing.T) {
	// Attempt <= 0 is clamped to 1.
	p := domain.RetryPolicy{MaxAttempts: 3, InitialDelay: 10 * time.Second, MaxDelay: time.Hour}
	now := time.Now()
	got := p.NextEligibleAt(0, now)
	want := now.Add(10 * time.Second)
	if got.Sub(now) != 10*time.Second {
		t.Errorf("attempt 0 clamped to 1: want %v, got %v", want, got)
	}
}
