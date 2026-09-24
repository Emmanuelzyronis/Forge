package domain_test

import (
	"testing"
	"time"

	"github.com/Emmanuelzyronis/forge/internal/domain"
	"github.com/google/uuid"
)

func newJob(state domain.JobState) *domain.Job {
	return &domain.Job{
		ID:          uuid.New(),
		Kind:        "test",
		State:       state,
		MaxAttempts: 3,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
		EligibleAt:  time.Now().UTC(),
	}
}

func TestJob_LegalTransitions(t *testing.T) {
	cases := []struct {
		from domain.JobState
		to   domain.JobState
	}{
		{domain.JobStateQueued, domain.JobStateClaimed},
		{domain.JobStateClaimed, domain.JobStateRunning},
		{domain.JobStateClaimed, domain.JobStateQueued},   // re-queue on lease expiry
		{domain.JobStateRunning, domain.JobStateSucceeded},
		{domain.JobStateRunning, domain.JobStateFailed},
		{domain.JobStateRunning, domain.JobStateTimedOut},
		{domain.JobStateRunning, domain.JobStateQueued},   // recovery re-queue
	}
	for _, tc := range cases {
		t.Run(string(tc.from)+"->"+string(tc.to), func(t *testing.T) {
			j := newJob(tc.from)
			if err := j.Transition(tc.to); err != nil {
				t.Fatalf("expected legal transition, got %v", err)
			}
			if j.State != tc.to {
				t.Fatalf("state not updated: got %s want %s", j.State, tc.to)
			}
		})
	}
}

func TestJob_IllegalTransitions(t *testing.T) {
	cases := []struct {
		from domain.JobState
		to   domain.JobState
	}{
		{domain.JobStateQueued, domain.JobStateRunning},
		{domain.JobStateQueued, domain.JobStateSucceeded},
		{domain.JobStateQueued, domain.JobStateFailed},
		{domain.JobStateQueued, domain.JobStateTimedOut},
		{domain.JobStateQueued, domain.JobStateQueued},
		{domain.JobStateClaimed, domain.JobStateSucceeded},
		{domain.JobStateClaimed, domain.JobStateFailed},
		{domain.JobStateClaimed, domain.JobStateTimedOut},
	}
	for _, tc := range cases {
		t.Run(string(tc.from)+"->"+string(tc.to), func(t *testing.T) {
			j := newJob(tc.from)
			err := j.Transition(tc.to)
			if err == nil {
				t.Fatalf("expected error for illegal transition %s→%s", tc.from, tc.to)
			}
			var ite *domain.IllegalTransitionError
			if !isIllegalTransitionError(err, &ite) {
				t.Fatalf("expected IllegalTransitionError, got %T: %v", err, err)
			}
		})
	}
}

func TestJob_TerminalStatesCannotTransition(t *testing.T) {
	terminals := []domain.JobState{
		domain.JobStateSucceeded,
		domain.JobStateFailed,
		domain.JobStateTimedOut,
	}
	targets := []domain.JobState{
		domain.JobStateQueued,
		domain.JobStateClaimed,
		domain.JobStateRunning,
		domain.JobStateSucceeded,
		domain.JobStateFailed,
		domain.JobStateTimedOut,
	}
	for _, term := range terminals {
		for _, target := range targets {
			t.Run(string(term)+"->"+string(target), func(t *testing.T) {
				j := newJob(term)
				err := j.Transition(target)
				if err == nil {
					t.Fatalf("expected error: terminal %s should not transition to %s", term, target)
				}
			})
		}
	}
}

func TestJob_IsTerminal(t *testing.T) {
	for state, wantTerminal := range map[domain.JobState]bool{
		domain.JobStateQueued:    false,
		domain.JobStateClaimed:   false,
		domain.JobStateRunning:   false,
		domain.JobStateSucceeded: true,
		domain.JobStateFailed:    true,
		domain.JobStateTimedOut:  true,
	} {
		j := newJob(state)
		if got := j.IsTerminal(); got != wantTerminal {
			t.Errorf("IsTerminal(%s) = %v, want %v", state, got, wantTerminal)
		}
	}
}

func isIllegalTransitionError(err error, out **domain.IllegalTransitionError) bool {
	if ite, ok := err.(*domain.IllegalTransitionError); ok {
		*out = ite
		return true
	}
	return false
}
