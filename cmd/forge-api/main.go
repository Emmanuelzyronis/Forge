package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/Emmanuelzyronis/forge/internal/api"
	"github.com/Emmanuelzyronis/forge/internal/config"
	"github.com/Emmanuelzyronis/forge/internal/dispatch"
	"github.com/Emmanuelzyronis/forge/internal/lifecycle"
	"github.com/Emmanuelzyronis/forge/internal/postgres"
	"github.com/Emmanuelzyronis/forge/internal/recovery"
	"github.com/Emmanuelzyronis/forge/internal/registration"
	"github.com/Emmanuelzyronis/forge/internal/submission"
	"github.com/Emmanuelzyronis/forge/internal/telemetry"
)

func main() {
	cfg := config.Load()
	log := telemetry.New()

	log.Info().Str("addr", cfg.ListenAddr).Msg("forge-api starting")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create connection pool")
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}
	log.Info().Msg("database connection established")

	jobRepo := postgres.NewJobRepo(pool)
	workerRepo := postgres.NewWorkerRepo(pool)
	attemptRepo := postgres.NewAttemptRepo(pool)
	eventRepo := postgres.NewEventRepo(pool)

	submitSvc := submission.NewService(jobRepo, log)
	registrationSvc := registration.NewService(workerRepo, log)
	dispatchSvc := dispatch.NewService(jobRepo, cfg.LeaseDuration, log)
	lifecycleSvc := lifecycle.NewService(pool, cfg.LeaseDuration, log)

	metrics := telemetry.NewMetrics()

	scheduler := recovery.NewScheduler(pool, cfg.RecoveryInterval, log)
	go scheduler.Run(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", api.HealthHandler(pool, log))
	mux.Handle("GET /metrics", promhttp.HandlerFor(metrics.Registry, promhttp.HandlerOpts{}))
	mux.Handle("POST /jobs", api.SubmitJobHandler(submitSvc, log, metrics))
	mux.Handle("POST /workers", api.RegisterWorkerHandler(registrationSvc, log))
	mux.Handle("POST /workers/{id}/heartbeat", api.WorkerHeartbeatHandler(registrationSvc, log))
	mux.Handle("DELETE /workers/{id}", api.WorkerOfflineHandler(registrationSvc, log))
	mux.Handle("POST /workers/{id}/claim", api.ClaimJobHandler(dispatchSvc, log, metrics))
	mux.Handle("POST /attempts/{id}/start", api.StartAttemptHandler(lifecycleSvc, log))
	mux.Handle("POST /attempts/{id}/succeed", api.SucceedAttemptHandler(lifecycleSvc, log, metrics))
	mux.Handle("POST /attempts/{id}/fail", api.FailAttemptHandler(lifecycleSvc, log))
	mux.Handle("POST /jobs/{id}/heartbeat", api.LifecycleJobHeartbeatHandler(lifecycleSvc, log))

	// Query endpoints (read-only).
	mux.Handle("GET /jobs", api.ListJobsHandler(jobRepo, log))
	mux.Handle("GET /jobs/{id}", api.GetJobHandler(jobRepo, log))
	mux.Handle("GET /jobs/{id}/attempts", api.ListAttemptsHandler(attemptRepo, log))
	mux.Handle("GET /jobs/{id}/events", api.ListEventsHandler(eventRepo, log))
	mux.Handle("GET /workers", api.ListWorkersHandler(workerRepo, log))
	mux.Handle("GET /workers/{id}", api.GetWorkerHandler(workerRepo, log))

	handler := api.LoggingMiddleware(mux, log, metrics)

	srv := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Info().Str("addr", cfg.ListenAddr).Msg("http server listening")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("http server error")
		}
	}()

	<-ctx.Done()
	log.Info().Msg("shutting down")

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Error().Err(err).Msg("shutdown error")
	}
}
