package api

import (
	"net/http"
	"strconv"

	"github.com/Emmanuelzyronis/forge/internal/domain"
	"github.com/Emmanuelzyronis/forge/internal/store"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// GetJobHandler handles GET /jobs/{id}.
func GetJobHandler(jobs store.JobStore, log zerolog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid job id")
			return
		}
		job, err := jobs.GetByID(r.Context(), id)
		if err != nil {
			log.Error().Err(err).Msg("get job")
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeJSON(w, http.StatusOK, job)
	})
}

// ListJobsHandler handles GET /jobs?state=&kind=&correlation_id=&limit=&offset=
func ListJobsHandler(jobs store.JobStore, log zerolog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		filter := store.JobFilter{}
		if v := q.Get("state"); v != "" {
			s := domain.JobState(v)
			filter.State = &s
		}
		if v := q.Get("kind"); v != "" {
			filter.Kind = &v
		}
		if v := q.Get("correlation_id"); v != "" {
			filter.CorrelationID = &v
		}
		if v := q.Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				filter.Limit = n
			}
		}
		if v := q.Get("offset"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				filter.Offset = n
			}
		}
		result, err := jobs.List(r.Context(), filter)
		if err != nil {
			log.Error().Err(err).Msg("list jobs")
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if result == nil {
			result = []*domain.Job{}
		}
		writeJSON(w, http.StatusOK, result)
	})
}

// ListAttemptsHandler handles GET /jobs/{id}/attempts.
func ListAttemptsHandler(attempts store.AttemptStore, log zerolog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid job id")
			return
		}
		result, err := attempts.ListByJobID(r.Context(), id)
		if err != nil {
			log.Error().Err(err).Msg("list attempts")
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if result == nil {
			result = []*domain.ExecutionAttempt{}
		}
		writeJSON(w, http.StatusOK, result)
	})
}

// ListEventsHandler handles GET /jobs/{id}/events.
func ListEventsHandler(events store.EventStore, log zerolog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid job id")
			return
		}
		result, err := events.ListByJobID(r.Context(), id)
		if err != nil {
			log.Error().Err(err).Msg("list events")
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if result == nil {
			result = []*domain.JobEvent{}
		}
		writeJSON(w, http.StatusOK, result)
	})
}

// GetWorkerHandler handles GET /workers/{id}.
func GetWorkerHandler(workers store.WorkerStore, log zerolog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid worker id")
			return
		}
		worker, err := workers.GetByID(r.Context(), id)
		if err != nil {
			log.Error().Err(err).Msg("get worker")
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeJSON(w, http.StatusOK, worker)
	})
}

// ListWorkersHandler handles GET /workers.
func ListWorkersHandler(workers store.WorkerStore, log zerolog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result, err := workers.ListActive(r.Context())
		if err != nil {
			log.Error().Err(err).Msg("list workers")
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if result == nil {
			result = []*domain.Worker{}
		}
		writeJSON(w, http.StatusOK, result)
	})
}
