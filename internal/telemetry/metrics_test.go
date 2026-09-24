package telemetry_test

import (
	"testing"

	"github.com/Emmanuelzyronis/forge/internal/telemetry"
)

func TestNewMetrics_RegistersWithoutPanic(t *testing.T) {
	m := telemetry.NewMetrics()
	if m.Registry == nil {
		t.Error("expected non-nil registry")
	}
	m.JobsClaimed.Inc()
	m.JobsSucceeded.Inc()
	m.JobsSubmitted.With(map[string]string{"kind": "email"}).Inc()
	m.JobsFailed.With(map[string]string{"outcome": "exhausted"}).Inc()
	m.JobsRecovered.With(map[string]string{"outcome": "requeued"}).Inc()
}

func TestNewMetrics_TwoRegistriesIndependent(t *testing.T) {
	// Each call creates a private registry — must not panic with duplicate registration.
	m1 := telemetry.NewMetrics()
	m2 := telemetry.NewMetrics()
	m1.JobsClaimed.Inc()
	m2.JobsClaimed.Inc()
}
