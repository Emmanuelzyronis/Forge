package domain

import (
	"time"

	"github.com/google/uuid"
)

// WorkerState represents the current lifecycle state of a worker process.
type WorkerState string

const (
	WorkerStateRegistered WorkerState = "REGISTERED"
	WorkerStateIdle       WorkerState = "IDLE"
	WorkerStateBusy       WorkerState = "BUSY"
	WorkerStateStale      WorkerState = "STALE"
	WorkerStateOffline    WorkerState = "OFFLINE"
)

// Worker represents a registered execution process capable of claiming and running jobs.
type Worker struct {
	ID              uuid.UUID
	Name            string
	Hostname        string
	PID             int
	State           WorkerState
	Capabilities    []string
	LastHeartbeatAt time.Time
	RegisteredAt    time.Time
	UpdatedAt       time.Time
	LastJobID       *uuid.UUID
}

// IsStale reports whether the worker has missed its heartbeat deadline.
func (w *Worker) IsStale(now time.Time, heartbeatWindow time.Duration) bool {
	return now.Sub(w.LastHeartbeatAt) > heartbeatWindow
}
