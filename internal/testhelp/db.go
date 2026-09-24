package testhelp

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// OpenDB opens a test database connection, skipping the test if FORGE_DATABASE_URL is unset.
func OpenDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("FORGE_DATABASE_URL")
	if dsn == "" {
		t.Skip("FORGE_DATABASE_URL not set; skipping integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// TruncateTables removes all rows from the core tables between tests.
func TruncateTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`TRUNCATE job_events, job_attempts, jobs, workers RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
}
