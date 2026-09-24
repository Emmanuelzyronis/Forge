package api

import (
	"encoding/json"
	"net/http"

	"github.com/Emmanuelzyronis/forge/internal/registration"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type registerWorkerRequest struct {
	Name         string   `json:"name"`
	Hostname     string   `json:"hostname"`
	PID          int      `json:"pid"`
	Capabilities []string `json:"capabilities"`
}

type workerResponse struct {
	WorkerID string `json:"worker_id"`
	Name     string `json:"name"`
	State    string `json:"state"`
}

// RegisterWorkerHandler handles POST /workers.
func RegisterWorkerHandler(svc *registration.Service, log zerolog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body registerWorkerRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		if body.Name == "" {
			writeError(w, http.StatusBadRequest, "name: required")
			return
		}

		worker, err := svc.Register(r.Context(), registration.RegisterRequest{
			Name:         body.Name,
			Hostname:     body.Hostname,
			PID:          body.PID,
			Capabilities: body.Capabilities,
		})
		if err != nil {
			log.Error().Err(err).Msg("worker registration failed")
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		writeJSON(w, http.StatusOK, workerResponse{
			WorkerID: worker.ID.String(),
			Name:     worker.Name,
			State:    string(worker.State),
		})
	}
}

// WorkerHeartbeatHandler handles POST /workers/{id}/heartbeat.
func WorkerHeartbeatHandler(svc *registration.Service, log zerolog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		workerID, err := uuid.Parse(idStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid worker id")
			return
		}

		if err := svc.Heartbeat(r.Context(), workerID); err != nil {
			log.Error().Err(err).Str("worker_id", idStr).Msg("heartbeat failed")
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// WorkerOfflineHandler handles DELETE /workers/{id}.
func WorkerOfflineHandler(svc *registration.Service, log zerolog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		workerID, err := uuid.Parse(idStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid worker id")
			return
		}

		if err := svc.Offline(r.Context(), workerID); err != nil {
			log.Error().Err(err).Str("worker_id", idStr).Msg("offline failed")
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
