package domain

import (
	"time"

	"github.com/google/uuid"
)

// EventType classifies a job lifecycle event for the immutable audit log.
type EventType string

const (
	EventTypeSubmitted  EventType = "SUBMITTED"
	EventTypeClaimed    EventType = "CLAIMED"
	EventTypeStarted    EventType = "STARTED"
	EventTypeSucceeded  EventType = "SUCCEEDED"
	EventTypeFailed     EventType = "FAILED"
	EventTypeTimedOut   EventType = "TIMED_OUT"
	EventTypeAbandoned  EventType = "ABANDONED"
	EventTypeRequeued   EventType = "REQUEUED"
	EventTypeHeartbeat  EventType = "HEARTBEAT"
)

// JobEvent is an immutable record of a state transition or lifecycle observation.
// Events are append-only; they are never updated or deleted (F-INV-010).
type JobEvent struct {
	ID            uuid.UUID
	JobID         uuid.UUID
	AttemptID     *uuid.UUID
	WorkerID      *uuid.UUID
	Type          EventType
	FromState     *string
	ToState       *string
	Metadata      []byte
	OccurredAt    time.Time
	CorrelationID *string
}
