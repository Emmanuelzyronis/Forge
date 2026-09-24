package api

import (
	"net/http"

	"github.com/Emmanuelzyronis/forge/internal/dispatch"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type claimResponse struct {
	JobID          string `json:"job_id"`
	AttemptID      string `json:"attempt_id"`
	AttemptNum     int    `json:"attempt_num"`
	Kind           string `json:"kind"`
	Payload        []byte `json:"payload"`
	MaxAttempts    int    `json:"max_attempts"`
	TimeoutSecs    int    `json:"timeout_secs"`
	LeaseToken     string `json:"lease_token"`
	LeaseExpiresAt string `json:"lease_expires_at"`
	CorrelationID  string `json:"correlation_id,omitempty"`
}

// ClaimJobHandler handles POST /workers/{id}/claim.
// Returns 200+job when a job is available, 204 when the queue is empty.
func ClaimJobHandler(svc *dispatch.Service, log zerolog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		workerID, err := uuid.Parse(idStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid worker id")
			return
		}

		result, err := svc.ClaimNext(r.Context(), workerID, nil)
		if err != nil {
			log.Error().Err(err).Str("worker_id", idStr).Msg("claim failed")
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if result == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		var correlationID string
		if result.Job.CorrelationID != nil {
			correlationID = *result.Job.CorrelationID
		}

		var leaseExpiresAt string
		if result.Job.LeaseExpiresAt != nil {
			leaseExpiresAt = result.Job.LeaseExpiresAt.Format("2006-01-02T15:04:05Z07:00")
		}

		writeJSON(w, http.StatusOK, claimResponse{
			JobID:          result.Job.ID.String(),
			AttemptID:      result.Attempt.ID.String(),
			AttemptNum:     result.Attempt.AttemptNum,
			Kind:           result.Job.Kind,
			Payload:        result.Job.Payload,
			MaxAttempts:    result.Job.MaxAttempts,
			TimeoutSecs:    result.Job.TimeoutSecs,
			LeaseToken:     result.LeaseToken.String(),
			LeaseExpiresAt: leaseExpiresAt,
			CorrelationID:  correlationID,
		})
	}
}
