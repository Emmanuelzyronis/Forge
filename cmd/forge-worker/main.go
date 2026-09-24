package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Emmanuelzyronis/forge/internal/config"
	"github.com/Emmanuelzyronis/forge/internal/telemetry"
	"github.com/Emmanuelzyronis/forge/internal/worker"
)

func main() {
	cfg := config.Load()
	log := telemetry.New()

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	log.Info().
		Str("worker", cfg.WorkerName).
		Str("api_url", cfg.APIURL).
		Str("hostname", hostname).
		Msg("forge-worker starting")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	client := worker.NewClient(cfg.APIURL)

	// Register with the API server. Retry until the API is available (safe in
	// Docker Compose where the worker may start before the API is ready).
	var reg *worker.RegisterResponse
	for {
		reg, err = client.Register(ctx, worker.RegisterRequest{
			Name:         cfg.WorkerName,
			Hostname:     hostname,
			PID:          os.Getpid(),
			Capabilities: []string{},
		})
		if err == nil {
			break
		}
		log.Warn().Err(err).Msg("registration failed; retrying in 2s")
		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
			log.Info().Msg("forge-worker shutting down before registration")
			return
		}
	}

	log.Info().
		Str("worker_id", reg.WorkerID).
		Str("name", reg.Name).
		Str("state", reg.State).
		Msg("registered")

	// Heartbeat loop — must fire before lease_expires_at so the recovery
	// scheduler does not reclaim in-flight jobs from a healthy worker.
	go func() {
		ticker := time.NewTicker(cfg.HeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := client.Heartbeat(ctx, reg.WorkerID); err != nil {
					log.Warn().Err(err).Msg("heartbeat failed")
				} else {
					log.Debug().Str("worker_id", reg.WorkerID).Msg("heartbeat sent")
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	poller := worker.NewPoller(
		reg.WorkerID,
		client,
		cfg.PollInterval,
		worker.StubExecutor(log),
		log,
	)
	go poller.Run(ctx)

	log.Info().Str("worker_id", reg.WorkerID).Msg("forge-worker ready")
	<-ctx.Done()

	log.Info().Msg("forge-worker shutting down")

	// Mark offline on clean shutdown (best-effort, 5s deadline).
	offCtx, offCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer offCancel()
	if err := client.Offline(offCtx, reg.WorkerID); err != nil {
		log.Warn().Err(err).Msg("offline notification failed")
	} else {
		log.Info().Msg("forge-worker offline")
	}
}
