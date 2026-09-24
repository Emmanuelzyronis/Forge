package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"

	"github.com/Emmanuelzyronis/forge/internal/telemetry"
)

// responseWriter wraps http.ResponseWriter to capture the status code and bytes written.
type responseWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytes += n
	return n, err
}

// LoggingMiddleware logs each request (method, path, status, duration) and sets X-Request-ID.
// When m is non-nil, it also records HTTP metrics.
func LoggingMiddleware(next http.Handler, log zerolog.Logger, m *telemetry.Metrics) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		reqID := uuid.New().String()
		w.Header().Set("X-Request-ID", reqID)

		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)

		duration := time.Since(start)
		path := r.Pattern // available in Go 1.22+

		log.Info().
			Str("request_id", reqID).
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Str("pattern", path).
			Int("status", rw.status).
			Dur("duration", duration).
			Int("bytes", rw.bytes).
			Msg("http request")

		if m != nil {
			labels := prometheus.Labels{
				"method": r.Method,
				"path":   path,
				"status": strconv.Itoa(rw.status),
			}
			m.HTTPRequests.With(labels).Inc()
			m.HTTPDuration.With(prometheus.Labels{
				"method": r.Method,
				"path":   path,
			}).Observe(duration.Seconds())
		}
	})
}
