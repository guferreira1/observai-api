package telemetry

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// ProviderMetrics is a Prometheus-backed implementation of observability.ProviderObserver.
type ProviderMetrics struct {
	duration *prometheus.HistogramVec
	failures *prometheus.CounterVec
}

// NewProviderMetrics registers per-adapter call duration and failure metrics.
func NewProviderMetrics(registry prometheus.Registerer) *ProviderMetrics {
	metrics := &ProviderMetrics{
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "observai_provider_call_duration_seconds",
			Help:    "Duration of outbound provider calls in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"provider", "operation", "outcome"}),
		failures: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "observai_provider_call_failures_total",
			Help: "Total failed outbound provider calls.",
		}, []string{"provider", "operation"}),
	}
	registry.MustRegister(metrics.duration, metrics.failures)
	return metrics
}

// Observe records the duration and outcome of a provider call.
func (metrics *ProviderMetrics) Observe(provider string, operation string, duration time.Duration, err error) {
	outcome := "success"
	if err != nil {
		outcome = "error"
		metrics.failures.WithLabelValues(provider, operation).Inc()
	}
	metrics.duration.WithLabelValues(provider, operation, outcome).Observe(duration.Seconds())
}
