package worker

import (
	"context"
	"time"

	"github.com/rs/zerolog"
)

// StubExecutor is a placeholder job handler used until Layer 7 wires in real
// execution lifecycle reporting. The job remains in CLAIMED state until the
// recovery scheduler reclaims it on lease expiry — demonstrating that path.
func StubExecutor(log zerolog.Logger) JobHandler {
	return func(ctx context.Context, job *ClaimedJob) error {
		log.Info().
			Str("job_id", job.JobID).
			Str("kind", job.Kind).
			Str("attempt_id", job.AttemptID).
			Msg("stub executor: simulating work")

		select {
		case <-time.After(500 * time.Millisecond):
			log.Info().Str("job_id", job.JobID).Msg("stub executor: done (no lifecycle reported — Layer 7)")
		case <-ctx.Done():
			log.Warn().Str("job_id", job.JobID).Msg("stub executor: interrupted by shutdown")
		}
		return nil
	}
}
