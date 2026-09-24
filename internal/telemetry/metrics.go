package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Metrics holds all Prometheus instruments for forge-api.
// A private registry (not prometheus.DefaultRegisterer) avoids test pollution.
type Metrics struct {
	HTTPRequests  *prometheus.CounterVec
	HTTPDuration  *prometheus.HistogramVec
	JobsSubmitted *prometheus.CounterVec
	JobsClaimed   prometheus.Counter
	JobsSucceeded prometheus.Counter
	JobsFailed    *prometheus.CounterVec
	JobsRecovered *prometheus.CounterVec
	JobDurationMs *prometheus.HistogramVec
	Registry      *prometheus.Registry
}

func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()
	m := &Metrics{
		HTTPRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "forge_http_requests_total",
			Help: "Total HTTP requests by method, path and status code.",
		}, []string{"method", "path", "status"}),
		HTTPDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "forge_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "path"}),
		JobsSubmitted: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "forge_jobs_submitted_total",
			Help: "Total jobs submitted, by kind.",
		}, []string{"kind"}),
		JobsClaimed: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "forge_jobs_claimed_total",
			Help: "Total job claims issued to workers.",
		}),
		JobsSucceeded: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "forge_jobs_succeeded_total",
			Help: "Total jobs that succeeded.",
		}),
		JobsFailed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "forge_jobs_failed_total",
			Help: "Total jobs that reached a terminal failure state.",
		}, []string{"outcome"}),
		JobsRecovered: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "forge_jobs_recovered_total",
			Help: "Total jobs processed by the recovery scheduler.",
		}, []string{"outcome"}),
		JobDurationMs: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "forge_job_duration_milliseconds",
			Help:    "Reported job execution duration in milliseconds.",
			Buckets: []float64{10, 50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000},
		}, []string{"kind"}),
		Registry: reg,
	}
	reg.MustRegister(
		m.HTTPRequests, m.HTTPDuration,
		m.JobsSubmitted, m.JobsClaimed, m.JobsSucceeded,
		m.JobsFailed, m.JobsRecovered, m.JobDurationMs,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return m
}
