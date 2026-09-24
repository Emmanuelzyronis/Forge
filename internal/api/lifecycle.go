package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Emmanuelzyronis/forge/internal/domain"
	"github.com/Emmanuelzyronis/forge/internal/lifecycle"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type startAttemptRequest struct {
	LeaseToken string `json:"lease_token"`
}

type succeedAttemptRequest struct {
	LeaseToken string          `json:"lease_token"`
	Result     json.RawMessage `json:"result"`
	DurationMS int64           `json:"duration_ms"`
}

type failAttemptRequest struct {
	LeaseToken string          `json:"lease_token"`
	Reason     string          `json:"reason"`
	Detail     json.RawMessage `json:"detail"`
}

type lifecycleHeartbeatRequest struct {
	LeaseToken string `json:"lease_token"`
}

// StartAttemptHandler handles POST /attempts/{id}/start.
func StartAttemptHandler(svc *lifecycle.Service, log zerolog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		attemptID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid attempt id")
			return
		}
		var body startAttemptRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		leaseToken, err := uuid.Parse(body.LeaseToken)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid lease_token")
			return
		}
		if err := svc.Start(r.Context(), attemptID, leaseToken); err != nil {
			if isStale(err) {
				writeError(w, http.StatusConflict, err.Error())
				return
			}
			log.Error().Err(err).Str("attempt_id", attemptID.String()).Msg("start attempt failed")
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
	}
}

// SucceedAttemptHandler handles POST /attempts/{id}/succeed.
func SucceedAttemptHandler(svc *lifecycle.Service, log zerolog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		attemptID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid attempt id")
			return
		}
		var body succeedAttemptRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		leaseToken, err := uuid.Parse(body.LeaseToken)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid lease_token")
			return
		}
		if err := svc.Succeed(r.Context(), attemptID, leaseToken, []byte(body.Result), body.DurationMS); err != nil {
			if isStale(err) {
				writeError(w, http.StatusConflict, err.Error())
				return
			}
			log.Error().Err(err).Str("attempt_id", attemptID.String()).Msg("succeed attempt failed")
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "succeeded"})
	}
}

// FailAttemptHandler handles POST /attempts/{id}/fail.
func FailAttemptHandler(svc *lifecycle.Service, log zerolog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		attemptID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid attempt id")
			return
		}
		var body failAttemptRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		leaseToken, err := uuid.Parse(body.LeaseToken)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid lease_token")
			return
		}
		if err := svc.Fail(r.Context(), attemptID, leaseToken, body.Reason, []byte(body.Detail)); err != nil {
			if isStale(err) {
				writeError(w, http.StatusConflict, err.Error())
				return
			}
			log.Error().Err(err).Str("attempt_id", attemptID.String()).Msg("fail attempt failed")
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "failed"})
	}
}

// LifecycleJobHeartbeatHandler handles POST /jobs/{id}/heartbeat (execution heartbeat,
// distinct from the worker registration heartbeat on /workers/{id}/heartbeat).
func LifecycleJobHeartbeatHandler(svc *lifecycle.Service, log zerolog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid job id")
			return
		}
		var body lifecycleHeartbeatRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		leaseToken, err := uuid.Parse(body.LeaseToken)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid lease_token")
			return
		}
		if err := svc.JobHeartbeat(r.Context(), jobID, leaseToken); err != nil {
			if isStale(err) {
				writeError(w, http.StatusConflict, err.Error())
				return
			}
			log.Error().Err(err).Str("job_id", jobID.String()).Msg("job lifecycle heartbeat failed")
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func isStale(err error) bool {
	var s *domain.StaleLeaseError
	return errors.As(err, &s)
}
