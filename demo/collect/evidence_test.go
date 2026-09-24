package main

import (
	"testing"
	"time"
)

func mustTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func strPtr(s string) *string { return &s }

// workers used across tests
var testWorkers = []APIWorker{
	{ID: "w-aaa", Name: "worker-01", State: "ONLINE"},
	{ID: "w-bbb", Name: "worker-02", State: "ONLINE"},
}

// ─── workerNameByID ───────────────────────────────────────────────────────────

func TestWorkerNameByIDFound(t *testing.T) {
	name := workerNameByID("w-aaa", testWorkers)
	if name != "worker-01" {
		t.Fatalf("want worker-01, got %q", name)
	}
}

func TestWorkerNameByIDNotFound(t *testing.T) {
	name := workerNameByID("w-zzz", testWorkers)
	// should return short-id fallback or the full id
	if name == "" {
		t.Fatal("expected non-empty fallback name")
	}
}

// ─── normalize ────────────────────────────────────────────────────────────────

func baseRaw() *RawEvidence {
	epoch := mustTime("2026-01-01T12:00:00Z")
	j := &APIJob{
		ID:          "job-001",
		Kind:        "demo.crash-recovery",
		State:       "SUCCEEDED",
		MaxAttempts: 3,
		AttemptCount: 2,
		CreatedAt:   epoch,
		UpdatedAt:   epoch.Add(60 * time.Second),
	}
	terminal := epoch.Add(60 * time.Second)
	j.TerminalAt = &terminal

	wid1 := "w-aaa"
	wid2 := "w-bbb"
	events := []APIEvent{
		{
			ID:         "ev-1",
			JobID:      "job-001",
			WorkerID:   nil,
			Type:       "SUBMITTED",
			ToState:    strPtr("QUEUED"),
			OccurredAt: epoch,
		},
		{
			ID:         "ev-2",
			JobID:      "job-001",
			WorkerID:   &wid1,
			Type:       "CLAIMED",
			FromState:  strPtr("QUEUED"),
			ToState:    strPtr("CLAIMED"),
			OccurredAt: epoch.Add(1 * time.Second),
		},
		{
			ID:         "ev-3",
			JobID:      "job-001",
			WorkerID:   &wid1,
			Type:       "STARTED",
			FromState:  strPtr("CLAIMED"),
			ToState:    strPtr("RUNNING"),
			OccurredAt: epoch.Add(2 * time.Second),
		},
		{
			ID:         "ev-4",
			JobID:      "job-001",
			WorkerID:   nil,
			Type:       "ABANDONED",
			FromState:  strPtr("RUNNING"),
			ToState:    strPtr("QUEUED"),
			OccurredAt: epoch.Add(40 * time.Second),
		},
		{
			ID:         "ev-5",
			JobID:      "job-001",
			WorkerID:   &wid2,
			Type:       "CLAIMED",
			FromState:  strPtr("QUEUED"),
			ToState:    strPtr("CLAIMED"),
			OccurredAt: epoch.Add(41 * time.Second),
		},
		{
			ID:         "ev-6",
			JobID:      "job-001",
			WorkerID:   &wid2,
			Type:       "STARTED",
			FromState:  strPtr("CLAIMED"),
			ToState:    strPtr("RUNNING"),
			OccurredAt: epoch.Add(42 * time.Second),
		},
		{
			ID:         "ev-7",
			JobID:      "job-001",
			WorkerID:   &wid2,
			Type:       "SUCCEEDED",
			FromState:  strPtr("RUNNING"),
			ToState:    strPtr("SUCCEEDED"),
			OccurredAt: epoch.Add(55 * time.Second),
		},
	}

	return &RawEvidence{
		CollectedAt: epoch.Add(60 * time.Second),
		RunID:       "test-run",
		Job:         j,
		Events:      events,
		Workers:     testWorkers,
	}
}

func TestNormalizeTimeline_OrderAndLength(t *testing.T) {
	raw := baseRaw()
	p := normalize(raw, 38)

	// 7 API events + 1 synthetic CRASH = 8 entries
	if len(p.Timeline) != 8 {
		t.Fatalf("want 8 timeline entries (7 events + 1 CRASH), got %d", len(p.Timeline))
	}

	for i := 1; i < len(p.Timeline); i++ {
		if p.Timeline[i].ElapsedSecs < p.Timeline[i-1].ElapsedSecs {
			t.Errorf("timeline not sorted at index %d: %.3f < %.3f",
				i, p.Timeline[i].ElapsedSecs, p.Timeline[i-1].ElapsedSecs)
		}
	}
}

func TestNormalizeTimeline_CrashInserted(t *testing.T) {
	raw := baseRaw()
	p := normalize(raw, 38)

	var crashEntry *TimelineEntry
	for i := range p.Timeline {
		if p.Timeline[i].IsCrash {
			crashEntry = &p.Timeline[i]
			break
		}
	}
	if crashEntry == nil {
		t.Fatal("expected a synthetic CRASH entry in timeline")
	}
	if crashEntry.EventType != "CRASH" {
		t.Errorf("crash entry EventType = %q, want CRASH", crashEntry.EventType)
	}
	if crashEntry.WorkerName != "worker-01" {
		t.Errorf("crash entry WorkerName = %q, want worker-01", crashEntry.WorkerName)
	}
}

func TestNormalizeTimeline_RecoveryTagged(t *testing.T) {
	raw := baseRaw()
	p := normalize(raw, 38)

	var foundRecovery bool
	for _, e := range p.Timeline {
		if e.IsRecovery {
			foundRecovery = true
			break
		}
	}
	if !foundRecovery {
		t.Fatal("expected at least one entry tagged IsRecovery=true")
	}
}

func TestNormalizeAttempts_Sorted(t *testing.T) {
	wid1 := "w-aaa"
	wid2 := "w-bbb"
	raw := baseRaw()
	raw.Attempts = []APIAttempt{
		{ID: "a-2", JobID: "job-001", WorkerID: &wid2, AttemptNum: 2, State: "SUCCEEDED"},
		{ID: "a-1", JobID: "job-001", WorkerID: &wid1, AttemptNum: 1, State: "ABANDONED"},
	}
	p := normalize(raw, 38)

	if len(p.Attempts) != 2 {
		t.Fatalf("want 2 attempts, got %d", len(p.Attempts))
	}
	if p.Attempts[0].AttemptNum != 1 {
		t.Errorf("attempts[0].AttemptNum = %d, want 1", p.Attempts[0].AttemptNum)
	}
	if p.Attempts[1].AttemptNum != 2 {
		t.Errorf("attempts[1].AttemptNum = %d, want 2", p.Attempts[1].AttemptNum)
	}
}

func TestNormalizeStats(t *testing.T) {
	raw := baseRaw()
	p := normalize(raw, 38)

	if p.Stats.RecoveryLatencySecs != 38 {
		t.Errorf("RecoveryLatencySecs = %d, want 38", p.Stats.RecoveryLatencySecs)
	}
	if p.Stats.LeaseDurationSecs != 30 {
		t.Errorf("LeaseDurationSecs = %d, want 30", p.Stats.LeaseDurationSecs)
	}
	if p.Stats.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", p.Stats.MaxAttempts)
	}
	if p.Stats.TotalDurationSecs != 60.0 {
		t.Errorf("TotalDurationSecs = %.1f, want 60.0", p.Stats.TotalDurationSecs)
	}
}

func TestNormalizeEmptyEvents(t *testing.T) {
	raw := baseRaw()
	raw.Events = nil
	p := normalize(raw, 0)

	// should not panic; timeline may be nil or empty
	if p == nil {
		t.Fatal("normalize returned nil")
	}
	if len(p.Timeline) != 0 {
		t.Errorf("expected empty timeline for empty events, got %d entries", len(p.Timeline))
	}
}

func TestNormalizeIncompleteEvidence_NoCrash(t *testing.T) {
	// Only SUBMITTED event — no ABANDONED, so no crash marker should be inserted
	epoch := mustTime("2026-01-01T12:00:00Z")
	raw := &RawEvidence{
		RunID: "partial-run",
		Job: &APIJob{ID: "job-x", MaxAttempts: 3},
		Events: []APIEvent{
			{
				ID: "ev-1", JobID: "job-x",
				Type: "SUBMITTED", ToState: strPtr("QUEUED"),
				OccurredAt: epoch,
			},
		},
		Workers: testWorkers,
	}
	p := normalize(raw, 0)

	for _, e := range p.Timeline {
		if e.IsCrash {
			t.Error("unexpected CRASH entry when no ABANDONED event present")
		}
	}
}

func TestNormalizeScenarioName(t *testing.T) {
	p := normalize(baseRaw(), 38)
	if p.Scenario != "worker-crash-recovery" {
		t.Errorf("Scenario = %q, want worker-crash-recovery", p.Scenario)
	}
}

func TestNormalizeWorkerNames(t *testing.T) {
	raw := baseRaw()
	p := normalize(raw, 38)

	// CLAIMED by worker-01 should appear as "worker-01" in timeline
	var foundW1, foundW2 bool
	for _, e := range p.Timeline {
		if e.WorkerName == "worker-01" {
			foundW1 = true
		}
		if e.WorkerName == "worker-02" {
			foundW2 = true
		}
	}
	if !foundW1 {
		t.Error("expected worker-01 in timeline")
	}
	if !foundW2 {
		t.Error("expected worker-02 in timeline")
	}
}
