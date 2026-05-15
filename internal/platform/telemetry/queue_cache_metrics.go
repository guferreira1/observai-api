package telemetry

import "github.com/prometheus/client_golang/prometheus"

// QueueCacheMetrics groups the queue/cache gauges and counters the rest
// of the API uses on top of the per-call duration/failure metrics emitted
// through ProviderObserver.
//
// Adapters call QueueDepth/Set, IncQueueJob and IncCache* without
// importing prometheus directly so they keep depending on a small platform
// interface.
type QueueCacheMetrics struct {
	queueDepth *prometheus.GaugeVec
	queueJobs  *prometheus.CounterVec
	cacheOps   *prometheus.CounterVec
}

// NewQueueCacheMetrics registers the queue/cache gauges and counters in the
// supplied registry.
func NewQueueCacheMetrics(registry prometheus.Registerer) *QueueCacheMetrics {
	metrics := &QueueCacheMetrics{
		queueDepth: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "observai_queue_depth",
			Help: "Current number of in-flight analysis jobs waiting in the queue.",
		}, []string{"queue"}),
		queueJobs: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "observai_queue_jobs_total",
			Help: "Cumulative analysis jobs accepted by the queue, broken down by outcome.",
		}, []string{"queue", "outcome"}),
		cacheOps: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "observai_cache_operations_total",
			Help: "Cumulative analysis-context cache lookups, broken down by outcome (hit/miss).",
		}, []string{"cache", "outcome"}),
	}
	registry.MustRegister(metrics.queueDepth, metrics.queueJobs, metrics.cacheOps)
	return metrics
}

// SetQueueDepth records the current depth of the supplied queue.
func (metrics *QueueCacheMetrics) SetQueueDepth(queue string, depth int) {
	if metrics == nil {
		return
	}
	metrics.queueDepth.WithLabelValues(queue).Set(float64(depth))
}

// IncQueueJob increments the queue jobs counter for the supplied outcome.
func (metrics *QueueCacheMetrics) IncQueueJob(queue, outcome string) {
	if metrics == nil {
		return
	}
	metrics.queueJobs.WithLabelValues(queue, outcome).Inc()
}

// IncCacheHit increments the cache hit counter.
func (metrics *QueueCacheMetrics) IncCacheHit(cache string) {
	if metrics == nil {
		return
	}
	metrics.cacheOps.WithLabelValues(cache, "hit").Inc()
}

// IncCacheMiss increments the cache miss counter.
func (metrics *QueueCacheMetrics) IncCacheMiss(cache string) {
	if metrics == nil {
		return
	}
	metrics.cacheOps.WithLabelValues(cache, "miss").Inc()
}
