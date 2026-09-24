package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	DatabaseURL       string
	ListenAddr        string
	LeaseDuration     time.Duration
	RecoveryInterval  time.Duration
	PollInterval      time.Duration
	HeartbeatInterval time.Duration
	WorkerName        string
	APIURL            string
}

func Load() Config {
	return Config{
		DatabaseURL:       getEnv("FORGE_DATABASE_URL", "postgres://forge:forge@localhost:5432/forge?sslmode=disable"),
		ListenAddr:        getEnv("FORGE_LISTEN_ADDR", ":8080"),
		LeaseDuration:     getDuration("FORGE_LEASE_DURATION_SECS", 30) * time.Second,
		RecoveryInterval:  getDuration("FORGE_RECOVERY_INTERVAL_SECS", 10) * time.Second,
		PollInterval:      getDuration("FORGE_POLL_INTERVAL_SECS", 1) * time.Second,
		HeartbeatInterval: getDuration("FORGE_HEARTBEAT_INTERVAL_SECS", 10) * time.Second,
		WorkerName:        getEnv("FORGE_WORKER_NAME", "worker-1"),
		APIURL:            getEnv("FORGE_API_URL", "http://localhost:8080"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getDuration(key string, fallbackSecs int64) time.Duration {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return time.Duration(n)
		}
	}
	return time.Duration(fallbackSecs)
}
