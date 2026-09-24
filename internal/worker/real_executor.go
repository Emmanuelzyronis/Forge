package worker

import (
	"context"
	"time"

	"github.com/rs/zerolog"
)

// RealExecutor produces a JobHandler that reports the full CLAIMED→RUNNING→SUCCEEDED
// lifecycle to the API. This replaces the Layer 6 stub executor.
func RealExecutor(client *Client, log zerolog.Logger) JobHandler {
	return func(ctx context.Context, job *ClaimedJob) error {
		log.Info().
			Str("job_id", job.JobID).
			Str("kind", job.Kind).
			Str("attempt_id", job.AttemptID).
			Int("attempt_num", job.AttemptNum).
			Msg("executor: starting attempt")

		// Transition CLAIMED → RUNNING by reporting StartAttempt.
		if err := client.StartAttempt(ctx, job.AttemptID, job.LeaseToken); err != nil {
			log.Error().Err(err).Str("attempt_id", job.AttemptID).Msg("executor: StartAttempt failed")
			return err
		}

		// Run the job under a per-job timeout.
		execCtx, cancel := context.WithTimeout(ctx, time.Duration(job.TimeoutSecs)*time.Second)
		defer cancel()

		start := time.Now()
		result, execErr := runJob(execCtx, job)
		durationMS := time.Since(start).Milliseconds()

		if execErr != nil {
			log.Warn().Err(execErr).Str("job_id", job.JobID).Msg("executor: job failed")
			// Use a short-lived context for the failure report so shutdown does not
			// prevent the API from recording the terminal state.
			reportCtx, reportCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer reportCancel()
			if err := client.FailAttempt(reportCtx, job.AttemptID, job.LeaseToken, execErr.Error(), nil); err != nil {
				log.Error().Err(err).Str("attempt_id", job.AttemptID).Msg("executor: FailAttempt report failed")
			}
			return execErr
		}

		if err := client.SucceedAttempt(ctx, job.AttemptID, job.LeaseToken, result, durationMS); err != nil {
			log.Error().Err(err).Str("attempt_id", job.AttemptID).Msg("executor: SucceedAttempt failed")
			return err
		}
		log.Info().
			Str("job_id", job.JobID).
			Int64("duration_ms", durationMS).
			Msg("executor: job succeeded")
		return nil
	}
}

// runJob executes the job and returns the result payload.
// The default implementation simulates work; real dispatch-on-kind logic lives here.
func runJob(ctx context.Context, job *ClaimedJob) ([]byte, error) {
	select {
	case <-time.After(100 * time.Millisecond):
		return []byte(`{"status":"ok","kind":"` + job.Kind + `"}`), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
