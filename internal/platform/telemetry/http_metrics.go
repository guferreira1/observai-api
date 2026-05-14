package telemetry

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
)

// HTTPMetrics records API request metrics.
type HTTPMetrics struct {
	registry *prometheus.Registry
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	errors   *prometheus.CounterVec
}

// NewHTTPMetrics creates HTTP metrics with a dedicated registry.
func NewHTTPMetrics() *HTTPMetrics {
	registry := prometheus.NewRegistry()

	metrics := &HTTPMetrics{
		registry: registry,
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "observai_http_requests_total",
			Help: "Total HTTP requests processed by ObservAI API.",
		}, []string{"method", "path", "status"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "observai_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "path", "status"}),
		errors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "observai_http_errors_total",
			Help: "Total HTTP error responses produced by ObservAI API.",
		}, []string{"method", "path", "status"}),
	}

	registry.MustRegister(metrics.requests, metrics.duration, metrics.errors)
	return metrics
}

// Registry returns the Prometheus registry used by the metrics collector.
func (metrics *HTTPMetrics) Registry() *prometheus.Registry {
	return metrics.registry
}

// Middleware records request count, latency and error metrics.
func (metrics *HTTPMetrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		startedAt := time.Now()
		recorder := &statusRecorder{
			ResponseWriter: writer,
			status:         http.StatusOK,
		}

		next.ServeHTTP(recorder, request)

		status := strconv.Itoa(recorder.status)
		path := routePattern(request)
		metrics.requests.WithLabelValues(request.Method, path, status).Inc()
		metrics.duration.WithLabelValues(request.Method, path, status).Observe(time.Since(startedAt).Seconds())
		if recorder.status >= http.StatusBadRequest {
			metrics.errors.WithLabelValues(request.Method, path, status).Inc()
		}
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (recorder *statusRecorder) WriteHeader(status int) {
	recorder.status = status
	recorder.ResponseWriter.WriteHeader(status)
}

func routePattern(request *http.Request) string {
	routeContext := chi.RouteContext(request.Context())
	if routeContext == nil {
		return request.URL.Path
	}

	pattern := routeContext.RoutePattern()
	if pattern == "" {
		return request.URL.Path
	}

	return pattern
}
