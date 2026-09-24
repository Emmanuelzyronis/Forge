package domain_test

import (
	"testing"
	"time"

	"github.com/Emmanuelzyronis/forge/internal/domain"
	"github.com/google/uuid"
)

func TestLease_IsExpired(t *testing.T) {
	now := time.Now().UTC()
	lease := domain.NewLease(30*time.Second, now)

	if lease.IsExpired(now) {
		t.Fatal("newly created lease should not be expired")
	}
	if !lease.IsExpired(now.Add(31 * time.Second)) {
		t.Fatal("lease should be expired 31s after creation with 30s duration")
	}
	// Exactly at expiry — expired.
	if !lease.IsExpired(now.Add(30 * time.Second)) {
		t.Fatal("lease should be expired at exact expiry instant")
	}
}

func TestLease_IsOwner(t *testing.T) {
	now := time.Now().UTC()
	lease := domain.NewLease(30*time.Second, now)

	if !lease.IsOwner(lease.Token) {
		t.Fatal("token should match itself")
	}
	if lease.IsOwner(uuid.New()) {
		t.Fatal("random token should not match")
	}
}

func TestLease_Validate_Valid(t *testing.T) {
	now := time.Now().UTC()
	lease := domain.NewLease(30*time.Second, now)

	if err := lease.Validate(lease.Token, now); err != nil {
		t.Fatalf("expected nil error for valid lease, got %v", err)
	}
}

func TestLease_Validate_ExpiredToken(t *testing.T) {
	now := time.Now().UTC()
	lease := domain.NewLease(30*time.Second, now)

	err := lease.Validate(lease.Token, now.Add(31*time.Second))
	if err == nil {
		t.Fatal("expected StaleLeaseError for expired lease")
	}
	if _, ok := err.(*domain.StaleLeaseError); !ok {
		t.Fatalf("expected *StaleLeaseError, got %T: %v", err, err)
	}
}

func TestLease_Validate_WrongToken(t *testing.T) {
	now := time.Now().UTC()
	lease := domain.NewLease(30*time.Second, now)

	err := lease.Validate(uuid.New(), now)
	if err == nil {
		t.Fatal("expected StaleLeaseError for wrong token")
	}
	if _, ok := err.(*domain.StaleLeaseError); !ok {
		t.Fatalf("expected *StaleLeaseError, got %T: %v", err, err)
	}
}

func TestLease_Renew_RotatesToken(t *testing.T) {
	now := time.Now().UTC()
	lease := domain.NewLease(30*time.Second, now)
	oldToken := lease.Token

	lease.Renew(60*time.Second, now)

	if lease.Token == oldToken {
		t.Fatal("Renew must rotate the lease token")
	}
	if lease.ExpiresAt != now.Add(60*time.Second) {
		t.Fatalf("ExpiresAt should be now+60s, got %v", lease.ExpiresAt)
	}
}

func TestLease_Renew_OldTokenInvalid(t *testing.T) {
	now := time.Now().UTC()
	lease := domain.NewLease(30*time.Second, now)
	oldToken := lease.Token

	lease.Renew(60*time.Second, now)

	// Old token must no longer validate.
	err := lease.Validate(oldToken, now)
	if err == nil {
		t.Fatal("old token must be rejected after Renew")
	}
}
