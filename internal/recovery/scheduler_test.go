package recovery_test

import (
	"context"
	"testing"
	"time"

	"github.com/Emmanuelzyronis/forge/internal/recovery"
	"github.com/rs/zerolog"
)

// TestSchedulerStopsOnContextCancel verifies Run exits promptly when ctx is cancelled.
// The ticker interval (100ms) is longer than the context timeout (50ms), so the
// ticker never fires and the nil pool is never dereferenced.
func TestSchedulerStopsOnContextCancel(t *testing.T) {
	sched := recovery.NewScheduler(nil, 100*time.Millisecond, zerolog.Nop())

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Run panicked: %v", r)
			}
			close(done)
		}()
		sched.Run(ctx)
	}()

	select {
	case <-done:
		// good
	case <-time.After(500 * time.Millisecond):
		t.Error("Run did not exit after context cancellation")
	}
}
