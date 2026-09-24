package domain

import (
	"time"

	"github.com/google/uuid"
)

// Lease represents a worker's exclusive ownership claim on a job (F-INV-004).
type Lease struct {
	Token     uuid.UUID
	ExpiresAt time.Time
}

// NewLease creates a new lease with a fresh token expiring at now+duration.
func NewLease(duration time.Duration, now time.Time) Lease {
	return Lease{
		Token:     uuid.New(),
		ExpiresAt: now.Add(duration),
	}
}

// IsExpired reports whether the lease has passed its expiry time.
func (l *Lease) IsExpired(now time.Time) bool {
	return !now.Before(l.ExpiresAt)
}

// IsOwner reports whether the given token matches the lease token.
func (l *Lease) IsOwner(token uuid.UUID) bool {
	return l.Token == token
}

// Validate returns a StaleLeaseError if the token does not match or the lease is expired.
func (l *Lease) Validate(token uuid.UUID, now time.Time) error {
	if !l.IsOwner(token) {
		return &StaleLeaseError{Reason: "token mismatch"}
	}
	if l.IsExpired(now) {
		return &StaleLeaseError{Reason: "lease expired"}
	}
	return nil
}

// Renew extends the lease by duration from now, rotating the token (F-INV-004).
func (l *Lease) Renew(duration time.Duration, now time.Time) {
	l.Token = uuid.New()
	l.ExpiresAt = now.Add(duration)
}
