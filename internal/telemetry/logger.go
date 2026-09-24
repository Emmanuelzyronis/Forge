package telemetry

import (
	"context"
	"os"

	"github.com/rs/zerolog"
)

func New() zerolog.Logger {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	return zerolog.New(os.Stdout).With().Timestamp().Logger()
}

func WithJob(ctx context.Context, jobID string) context.Context {
	log := zerolog.Ctx(ctx).With().Str("job_id", jobID).Logger()
	return log.WithContext(ctx)
}

func WithAttempt(ctx context.Context, attemptID string) context.Context {
	log := zerolog.Ctx(ctx).With().Str("attempt_id", attemptID).Logger()
	return log.WithContext(ctx)
}

func WithWorker(ctx context.Context, workerID string) context.Context {
	log := zerolog.Ctx(ctx).With().Str("worker_id", workerID).Logger()
	return log.WithContext(ctx)
}
