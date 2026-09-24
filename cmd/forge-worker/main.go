package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/Emmanuelzyronis/forge/internal/config"
	"github.com/Emmanuelzyronis/forge/internal/telemetry"
)

func main() {
	cfg := config.Load()
	log := telemetry.New()

	log.Info().Str("worker", cfg.WorkerName).Str("api_url", cfg.APIURL).Msg("forge-worker starting")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	<-ctx.Done()
	log.Info().Msg("forge-worker shutting down")
}
