package domain_test

import (
	"testing"
	"time"

	"github.com/Emmanuelzyronis/forge/internal/domain"
	"github.com/google/uuid"
)

func newAttempt(state domain.AttemptState) *domain.ExecutionAttempt {
	return &domain.ExecutionAttempt{
		ID:         uuid.New(),
		JobID:      uuid.New(),
		WorkerID:   uuid.New(),
		AttemptNum: 1,
		State:      state,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
}

func TestAttempt_LegalTransitions(t *testing.T) {
	cases := []struct {
		from domain.AttemptState
		to   domain.AttemptState
	}{
		{domain.AttemptStateCreated, domain.AttemptStateRunning},
		{domain.AttemptStateRunning, domain.AttemptStateSucceeded},
		{domain.AttemptStateRunning, domain.AttemptStateFailed},
		{domain.AttemptStateRunning, domain.AttemptStateTimedOut},
		{domain.AttemptStateRunning, domain.AttemptStateAbandoned},
	}
	for _, tc := range cases {
		t.Run(string(tc.from)+"->"+string(tc.to), func(t *testing.T) {
			a := newAttempt(tc.from)
			if err := a.Transition(tc.to); err != nil {
				t.Fatalf("expected legal transition, got %v", err)
			}
			if a.State != tc.to {
				t.Fatalf("state not updated: got %s want %s", a.State, tc.to)
			}
		})
	}
}

func TestAttempt_IllegalTransitions(t *testing.T) {
	cases := []struct {
		from domain.AttemptState
		to   domain.AttemptState
	}{
		{domain.AttemptStateCreated, domain.AttemptStateSucceeded},
		{domain.AttemptStateCreated, domain.AttemptStateFailed},
		{domain.AttemptStateCreated, domain.AttemptStateAbandoned},
		{domain.AttemptStateCreated, domain.AttemptStateTimedOut},
	}
	for _, tc := range cases {
		t.Run(string(tc.from)+"->"+string(tc.to), func(t *testing.T) {
			a := newAttempt(tc.from)
			if err := a.Transition(tc.to); err == nil {
				t.Fatalf("expected error for illegal transition %s→%s", tc.from, tc.to)
			}
		})
	}
}

func TestAttempt_TerminalStatesCannotTransition(t *testing.T) {
	terminals := []domain.AttemptState{
		domain.AttemptStateSucceeded,
		domain.AttemptStateFailed,
		domain.AttemptStateTimedOut,
		domain.AttemptStateAbandoned,
	}
	targets := []domain.AttemptState{
		domain.AttemptStateCreated,
		domain.AttemptStateRunning,
		domain.AttemptStateSucceeded,
		domain.AttemptStateFailed,
	}
	for _, term := range terminals {
		for _, target := range targets {
			t.Run(string(term)+"->"+string(target), func(t *testing.T) {
				a := newAttempt(term)
				if err := a.Transition(target); err == nil {
					t.Fatalf("expected error: terminal %s should not transition to %s", term, target)
				}
			})
		}
	}
}

func TestAttempt_IsTerminal(t *testing.T) {
	for state, wantTerminal := range map[domain.AttemptState]bool{
		domain.AttemptStateCreated:   false,
		domain.AttemptStateRunning:   false,
		domain.AttemptStateSucceeded: true,
		domain.AttemptStateFailed:    true,
		domain.AttemptStateTimedOut:  true,
		domain.AttemptStateAbandoned: true,
	} {
		a := newAttempt(state)
		if got := a.IsTerminal(); got != wantTerminal {
			t.Errorf("IsTerminal(%s) = %v, want %v", state, got, wantTerminal)
		}
	}
}
