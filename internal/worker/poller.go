package worker

import (
	"context"
	"time"

	"github.com/rs/zerolog"
)

// JobHandler is called when a job is claimed. Layer 7 wires the real executor here.
// If it returns an error the poller logs it but continues running.
type JobHandler func(ctx context.Context, job *ClaimedJob) error

// Poller polls the API for claimable jobs on a fixed interval.
// It calls handler for each claimed job and waits for the handler to return
// before polling again — one job at a time per worker process.
type Poller struct {
	workerID     string
	client       *Client
	pollInterval time.Duration
	handler      JobHandler
	log          zerolog.Logger
}

func NewPoller(workerID string, client *Client, pollInterval time.Duration, handler JobHandler, log zerolog.Logger) *Poller {
	return &Poller{
		workerID:     workerID,
		client:       client,
		pollInterval: pollInterval,
		handler:      handler,
		log:          log,
	}
}

// Run polls until ctx is cancelled.
func (p *Poller) Run(ctx context.Context) {
	ticker := time.NewTicker(p.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.poll(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (p *Poller) poll(ctx context.Context) {
	job, err := p.client.Claim(ctx, p.workerID)
	if err != nil {
		if ctx.Err() != nil {
			return // shutting down
		}
		p.log.Warn().Err(err).Msg("claim error")
		return
	}
	if job == nil {
		p.log.Debug().Msg("queue empty")
		return
	}
	p.log.Info().
		Str("job_id", job.JobID).
		Str("kind", job.Kind).
		Int("attempt_num", job.AttemptNum).
		Msg("job claimed")

	if err := p.handler(ctx, job); err != nil {
		p.log.Error().Err(err).Str("job_id", job.JobID).Msg("job handler error")
	}
}
