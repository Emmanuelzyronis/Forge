package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/Emmanuelzyronis/forge/internal/submission"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// submitJobRequest is the JSON body for POST /jobs.
type submitJobRequest struct {
	IdempotencyKey string          `json:"idempotency_key"`
	Kind           string          `json:"kind"`
	Payload        json.RawMessage `json:"payload"`
	Priority       int             `json:"priority"`
	MaxAttempts    int             `json:"max_attempts"`
	TimeoutSecs    int             `json:"timeout_secs"`
	EligibleAt     *time.Time      `json:"eligible_at"`
	CorrelationID  string          `json:"correlation_id"`
}

// jobResponse is the JSON body returned for a created or existing job.
type jobResponse struct {
	JobID          uuid.UUID `json:"job_id"`
	IdempotencyKey string    `json:"idempotency_key,omitempty"`
	Kind           string    `json:"kind"`
	State          string    `json:"state"`
	Priority       int       `json:"priority"`
	MaxAttempts    int       `json:"max_attempts"`
	AttemptCount   int       `json:"attempt_count"`
	TimeoutSecs    int       `json:"timeout_secs"`
	CreatedAt      time.Time `json:"created_at"`
	EligibleAt     time.Time `json:"eligible_at"`
	CorrelationID  string    `json:"correlation_id,omitempty"`
}

// SubmitJobHandler returns a handler for POST /jobs.
func SubmitJobHandler(svc *submission.Service, log zerolog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body submitJobRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}

		req := submission.SubmitRequest{
			IdempotencyKey: body.IdempotencyKey,
			Kind:           body.Kind,
			Payload:        body.Payload,
			Priority:       body.Priority,
			MaxAttempts:    body.MaxAttempts,
			TimeoutSecs:    body.TimeoutSecs,
			EligibleAt:     body.EligibleAt,
			CorrelationID:  body.CorrelationID,
		}

		result, err := svc.Submit(r.Context(), req)
		if err != nil {
			var ve *submission.ValidationError
			if errors.As(err, &ve) {
				writeError(w, http.StatusBadRequest, ve.Error())
				return
			}
			log.Error().Err(err).Msg("submit job failed")
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		status := http.StatusCreated
		if !result.Created {
			status = http.StatusOK
		}

		var idempotencyKey, correlationID string
		if result.Job.IdempotencyKey != nil {
			idempotencyKey = *result.Job.IdempotencyKey
		}
		if result.Job.CorrelationID != nil {
			correlationID = *result.Job.CorrelationID
		}

		resp := jobResponse{
			JobID:          result.Job.ID,
			IdempotencyKey: idempotencyKey,
			Kind:           result.Job.Kind,
			State:          string(result.Job.State),
			Priority:       result.Job.Priority,
			MaxAttempts:    result.Job.MaxAttempts,
			AttemptCount:   result.Job.AttemptCount,
			TimeoutSecs:    result.Job.TimeoutSecs,
			CreatedAt:      result.Job.CreatedAt,
			EligibleAt:     result.Job.EligibleAt,
			CorrelationID:  correlationID,
		}
		writeJSON(w, status, resp)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
