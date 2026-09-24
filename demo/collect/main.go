// Package main is the FORGE evidence collector.
//
// It calls the FORGE API to retrieve the complete lifecycle of a demo job,
// writes raw-evidence.json, then normalizes that into presentation.json
// which Remotion consumes to render the portfolio video.
//
// Usage:
//
//	go run ./demo/collect \
//	  --base-url http://localhost:8081 \
//	  --job-id <uuid> \
//	  --out-dir demo/evidence \
//	  --run-id demo-crash-1234567890 \
//	  --recovery-latency-secs 38
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"time"
)

// ─── API response shapes (matches FORGE domain types as serialised by pgx) ───

type APIJob struct {
	ID             string     `json:"ID"`
	Kind           string     `json:"Kind"`
	State          string     `json:"State"`
	AttemptCount   int        `json:"AttemptCount"`
	MaxAttempts    int        `json:"MaxAttempts"`
	CreatedAt      time.Time  `json:"CreatedAt"`
	UpdatedAt      time.Time  `json:"UpdatedAt"`
	TerminalAt     *time.Time `json:"TerminalAt"`
	CorrelationID  *string    `json:"CorrelationID"`
	IdempotencyKey *string    `json:"IdempotencyKey"`
}

type APIEvent struct {
	ID            string     `json:"ID"`
	JobID         string     `json:"JobID"`
	AttemptID     *string    `json:"AttemptID"`
	WorkerID      *string    `json:"WorkerID"`
	Type          string     `json:"Type"`
	FromState     *string    `json:"FromState"`
	ToState       *string    `json:"ToState"`
	Metadata      any        `json:"Metadata"`
	OccurredAt    time.Time  `json:"OccurredAt"`
	CorrelationID *string    `json:"CorrelationID"`
}

type APIAttempt struct {
	ID          string     `json:"ID"`
	JobID       string     `json:"JobID"`
	WorkerID    *string    `json:"WorkerID"`
	AttemptNum  int        `json:"AttemptNum"`
	State       string     `json:"State"`
	StartedAt   *time.Time `json:"StartedAt"`
	FinishedAt  *time.Time `json:"FinishedAt"`
	DurationMS  *int64     `json:"DurationMS"`
	FailReason  *string    `json:"FailReason"`
}

type APIWorker struct {
	ID       string     `json:"ID"`
	Name     string     `json:"Name"`
	State    string     `json:"State"`
	LastSeen *time.Time `json:"LastSeen"`
}

// ─── Raw evidence ─────────────────────────────────────────────────────────────

type RawEvidence struct {
	CollectedAt time.Time    `json:"collected_at"`
	RunID       string       `json:"run_id"`
	BaseURL     string       `json:"base_url"`
	Job         *APIJob      `json:"job"`
	Events      []APIEvent   `json:"events"`
	Attempts    []APIAttempt `json:"attempts"`
	Workers     []APIWorker  `json:"workers"`
}

// ─── Presentation (consumed by Remotion) ──────────────────────────────────────

type TimelineEntry struct {
	ElapsedSecs    float64 `json:"elapsed_secs"`
	EventType      string  `json:"event_type,omitempty"`
	FromState      string  `json:"from_state,omitempty"`
	ToState        string  `json:"to_state,omitempty"`
	WorkerName     string  `json:"worker_name,omitempty"`
	AttemptNum     int     `json:"attempt_num,omitempty"`
	IsCrash        bool    `json:"is_crash,omitempty"`
	IsRecovery     bool    `json:"is_recovery,omitempty"`
}

type PresentationAttempt struct {
	AttemptNum int     `json:"attempt_num"`
	WorkerName string  `json:"worker_name"`
	State      string  `json:"state"`
	DurationMS *int64  `json:"duration_ms,omitempty"`
	FailReason string  `json:"fail_reason,omitempty"`
}

type PresentationStats struct {
	TotalDurationSecs    float64 `json:"total_duration_secs"`
	RecoveryLatencySecs  int     `json:"recovery_latency_secs"`
	LeaseDurationSecs    int     `json:"lease_duration_secs"`
	RecoveryIntervalSecs int     `json:"recovery_interval_secs"`
	AttemptCount         int     `json:"attempt_count"`
	MaxAttempts          int     `json:"max_attempts"`
}

type Presentation struct {
	Scenario        string                `json:"scenario"`
	GeneratedAt     time.Time             `json:"generated_at"`
	RunID           string                `json:"run_id"`
	Job             *APIJob               `json:"job"`
	Timeline        []TimelineEntry       `json:"timeline"`
	Attempts        []PresentationAttempt `json:"attempts"`
	Workers         []APIWorker           `json:"workers"`
	Stats           PresentationStats     `json:"stats"`
}

// ─── HTTP helpers ─────────────────────────────────────────────────────────────

func get(baseURL, path string, out any) error {
	resp, err := http.Get(baseURL + path) //nolint:noctx
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GET %s: status %d: %s", path, resp.StatusCode, body)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// ─── Normalization ────────────────────────────────────────────────────────────

// workerNameByID returns a human-readable worker name from the worker list,
// or a short ID suffix if not found.
func workerNameByID(id string, workers []APIWorker) string {
	for _, w := range workers {
		if w.ID == id {
			return w.Name
		}
	}
	if len(id) >= 8 {
		return "worker-" + id[:8]
	}
	return id
}

func normalize(raw *RawEvidence, recoveryLatencySecs int) *Presentation {
	if len(raw.Events) == 0 {
		return &Presentation{
			Scenario:    "worker-crash-recovery",
			GeneratedAt: time.Now().UTC(),
			RunID:       raw.RunID,
			Job:         raw.Job,
		}
	}

	sort.Slice(raw.Events, func(i, j int) bool {
		return raw.Events[i].OccurredAt.Before(raw.Events[j].OccurredAt)
	})

	epoch := raw.Events[0].OccurredAt

	// Find the elapsed time of the first STARTED event so the synthetic CRASH
	// marker can be placed a few seconds after execution began, not at t=0.
	startedElapsed := -1.0
	for _, ev := range raw.Events {
		if ev.Type == "STARTED" {
			startedElapsed = ev.OccurredAt.Sub(epoch).Seconds()
			break
		}
	}

	var timeline []TimelineEntry
	var crashInserted bool

	for _, ev := range raw.Events {
		elapsed := ev.OccurredAt.Sub(epoch).Seconds()

		workerName := ""
		if ev.WorkerID != nil {
			workerName = workerNameByID(*ev.WorkerID, raw.Workers)
		}
		fromState := ""
		if ev.FromState != nil {
			fromState = *ev.FromState
		}
		toState := ""
		if ev.ToState != nil {
			toState = *ev.ToState
		}

		entry := TimelineEntry{
			ElapsedSecs: elapsed,
			EventType:   ev.Type,
			FromState:   fromState,
			ToState:     toState,
			WorkerName:  workerName,
		}

		// Tag recovery re-queue events
		if ev.Type == "REQUEUED" || (ev.Type == "ABANDONED" && toState == "QUEUED") {
			entry.IsRecovery = true
		}

		// Insert synthetic crash marker before the ABANDONED event.
		// Place it 3s after STARTED (the demo script kills the worker within
		// a few seconds of observing RUNNING state), or 1/8 of the way between
		// STARTED and ABANDONED when the window is larger.
		if !crashInserted && ev.Type == "ABANDONED" {
			var crashElapsed float64
			if startedElapsed >= 0 {
				gap := elapsed - startedElapsed
				crashElapsed = startedElapsed + min(3.0, gap*0.12)
			} else {
				// fallback: 30s before abandoned (lease default)
				crashElapsed = elapsed - 30.0
				if crashElapsed < 0 {
					crashElapsed = 0
				}
			}
			// resolve worker-01 name from first STARTED event
			crashWorker := "worker-01"
			for _, e2 := range raw.Events {
				if e2.Type == "STARTED" && e2.WorkerID != nil {
					crashWorker = workerNameByID(*e2.WorkerID, raw.Workers)
					break
				}
			}
			timeline = append(timeline, TimelineEntry{
				ElapsedSecs: crashElapsed,
				EventType:   "CRASH",
				WorkerName:  crashWorker,
				IsCrash:     true,
			})
			crashInserted = true
		}

		timeline = append(timeline, entry)
	}

	// Build attempt summaries
	var attempts []PresentationAttempt
	sort.Slice(raw.Attempts, func(i, j int) bool {
		return raw.Attempts[i].AttemptNum < raw.Attempts[j].AttemptNum
	})
	for _, a := range raw.Attempts {
		wName := ""
		if a.WorkerID != nil {
			wName = workerNameByID(*a.WorkerID, raw.Workers)
		}
		failReason := ""
		if a.FailReason != nil {
			failReason = *a.FailReason
		}
		attempts = append(attempts, PresentationAttempt{
			AttemptNum: a.AttemptNum,
			WorkerName: wName,
			State:      a.State,
			DurationMS: a.DurationMS,
			FailReason: failReason,
		})
	}

	totalDuration := 0.0
	if raw.Job != nil && raw.Job.TerminalAt != nil {
		totalDuration = raw.Job.TerminalAt.Sub(epoch).Seconds()
	} else if len(timeline) > 0 {
		totalDuration = timeline[len(timeline)-1].ElapsedSecs
	}

	maxAttempts := 3
	if raw.Job != nil {
		maxAttempts = raw.Job.MaxAttempts
	}

	return &Presentation{
		Scenario:    "worker-crash-recovery",
		GeneratedAt: time.Now().UTC(),
		RunID:       raw.RunID,
		Job:         raw.Job,
		Timeline:    timeline,
		Attempts:    attempts,
		Workers:     raw.Workers,
		Stats: PresentationStats{
			TotalDurationSecs:    totalDuration,
			RecoveryLatencySecs:  recoveryLatencySecs,
			LeaseDurationSecs:    30,
			RecoveryIntervalSecs: 10,
			AttemptCount:         len(raw.Attempts),
			MaxAttempts:          maxAttempts,
		},
	}
}

// ─── Main ─────────────────────────────────────────────────────────────────────

func writeJSON(path string, v any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func main() {
	baseURL := flag.String("base-url", "http://localhost:8081", "FORGE API base URL")
	jobID := flag.String("job-id", "", "job UUID to collect evidence for (required)")
	outDir := flag.String("out-dir", "demo/evidence", "output directory")
	runID := flag.String("run-id", "", "demo run identifier")
	recoveryLatencyStr := flag.String("recovery-latency-secs", "0", "observed recovery latency in seconds")
	flag.Parse()

	if *jobID == "" {
		fmt.Fprintln(os.Stderr, "error: --job-id is required")
		os.Exit(1)
	}

	recoveryLatency, err := strconv.Atoi(*recoveryLatencyStr)
	if err != nil {
		recoveryLatency = 0
	}

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir %s: %v\n", *outDir, err)
		os.Exit(1)
	}

	fmt.Printf("collecting evidence for job %s …\n", *jobID)

	var job APIJob
	if err := get(*baseURL, "/jobs/"+*jobID, &job); err != nil {
		fmt.Fprintf(os.Stderr, "get job: %v\n", err)
		os.Exit(1)
	}

	var events []APIEvent
	if err := get(*baseURL, "/jobs/"+*jobID+"/events", &events); err != nil {
		fmt.Fprintf(os.Stderr, "get events: %v\n", err)
		os.Exit(1)
	}

	var attempts []APIAttempt
	if err := get(*baseURL, "/jobs/"+*jobID+"/attempts", &attempts); err != nil {
		fmt.Fprintf(os.Stderr, "get attempts: %v\n", err)
		os.Exit(1)
	}

	var workers []APIWorker
	if err := get(*baseURL, "/workers", &workers); err != nil {
		fmt.Fprintf(os.Stderr, "get workers: %v\n", err)
		os.Exit(1)
	}

	raw := &RawEvidence{
		CollectedAt: time.Now().UTC(),
		RunID:       *runID,
		BaseURL:     *baseURL,
		Job:         &job,
		Events:      events,
		Attempts:    attempts,
		Workers:     workers,
	}

	rawPath := *outDir + "/raw-evidence.json"
	if err := writeJSON(rawPath, raw); err != nil {
		fmt.Fprintf(os.Stderr, "write raw evidence: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  wrote %s\n", rawPath)

	presentation := normalize(raw, recoveryLatency)
	presPath := *outDir + "/presentation.json"
	if err := writeJSON(presPath, presentation); err != nil {
		fmt.Fprintf(os.Stderr, "write presentation: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  wrote %s\n", presPath)
	fmt.Printf("  timeline entries: %d\n", len(presentation.Timeline))
	fmt.Printf("  attempts: %d\n", len(presentation.Attempts))
}
