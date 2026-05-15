package telemetry

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// ProviderMetrics is a Prometheus-backed implementation of observability.ProviderObserver.
type ProviderMetrics struct {
	duration *prometheus.HistogramVec
	failures *prometheus.CounterVec
	tokens   *prometheus.CounterVec
	cost     *prometheus.CounterVec
}

// NewProviderMetrics registers per-adapter call duration, failure and (LLM)
// token usage metrics.
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
		tokens: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "observai_llm_tokens_used_total",
			Help: "Cumulative tokens reported by the LLM adapter, broken down by direction (prompt/completion).",
		}, []string{"provider", "operation", "direction"}),
		cost: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "observai_llm_cost_usd_total",
			Help: "Cumulative USD cost reported by the LLM adapter (when known).",
		}, []string{"provider", "operation"}),
	}
	registry.MustRegister(metrics.duration, metrics.failures, metrics.tokens, metrics.cost)
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

// RecordLLMUsage records token counts (prompt + completion) and an optional
// USD cost for a single LLM call. Negative or zero counts are ignored so
// adapters that cannot report usage just skip the call.
func (metrics *ProviderMetrics) RecordLLMUsage(provider, operation string, promptTokens, completionTokens int, costUSD float64) {
	if metrics.tokens == nil {
		return
	}
	if promptTokens > 0 {
		metrics.tokens.WithLabelValues(provider, operation, "prompt").Add(float64(promptTokens))
	}
	if completionTokens > 0 {
		metrics.tokens.WithLabelValues(provider, operation, "completion").Add(float64(completionTokens))
	}
	if costUSD > 0 {
		metrics.cost.WithLabelValues(provider, operation).Add(costUSD)
	}
}
