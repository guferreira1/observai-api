package http

import (
	stdhttp "net/http"
	"strings"
	"sync"
	"time"
)

const (
	rateLimiterMaxIdleBuckets = 10000
	rateLimiterEvictAfter     = 10 * time.Minute
)

// RateLimitConfig configures the per-IP HTTP rate limiter.
type RateLimitConfig struct {
	RequestsPerSecond float64
	Burst             int
}

// Enabled reports whether the configuration produces an active limiter.
func (cfg RateLimitConfig) Enabled() bool {
	return cfg.RequestsPerSecond > 0 && cfg.Burst > 0
}

type ipBucket struct {
	tokens   float64
	lastSeen time.Time
}

// rateLimiter implements a simple per-key token bucket. Buckets are evicted
// after rateLimiterEvictAfter inactivity once the map crosses the soft cap so
// that the limiter cannot leak memory under churn.
type rateLimiter struct {
	perSecond float64
	burst     float64
	mu        sync.Mutex
	buckets   map[string]*ipBucket
	now       func() time.Time
}

func newRateLimiter(cfg RateLimitConfig) *rateLimiter {
	if !cfg.Enabled() {
		return nil
	}
	return &rateLimiter{
		perSecond: cfg.RequestsPerSecond,
		burst:     float64(cfg.Burst),
		buckets:   make(map[string]*ipBucket),
		now:       time.Now,
	}
}

// Allow consumes a token for the provided key, returning whether the request
// is permitted. Empty keys are always allowed so the middleware fails open if
// the IP cannot be resolved.
func (limiter *rateLimiter) Allow(key string) bool {
	if key == "" {
		return true
	}

	now := limiter.now()
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	if len(limiter.buckets) > rateLimiterMaxIdleBuckets {
		limiter.evictStale(now)
	}

	bucket, exists := limiter.buckets[key]
	if !exists {
		bucket = &ipBucket{tokens: limiter.burst, lastSeen: now}
		limiter.buckets[key] = bucket
	} else {
		elapsed := now.Sub(bucket.lastSeen).Seconds()
		bucket.tokens += elapsed * limiter.perSecond
		if bucket.tokens > limiter.burst {
			bucket.tokens = limiter.burst
		}
		bucket.lastSeen = now
	}

	if bucket.tokens >= 1 {
		bucket.tokens--
		return true
	}
	return false
}

func (limiter *rateLimiter) evictStale(now time.Time) {
	for key, bucket := range limiter.buckets {
		if now.Sub(bucket.lastSeen) > rateLimiterEvictAfter {
			delete(limiter.buckets, key)
		}
	}
}

// rateLimitMiddleware enforces per-IP token-bucket throttling.
func rateLimitMiddleware(limiter *rateLimiter, provider providerSummaryProvider) func(stdhttp.Handler) stdhttp.Handler {
	if limiter == nil {
		return func(next stdhttp.Handler) stdhttp.Handler { return next }
	}
	return func(next stdhttp.Handler) stdhttp.Handler {
		return stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
			if !limiter.Allow(clientIP(request)) {
				writeRateLimitedResponse(writer, requestIDFromContext(request.Context()), middlewareProviderSummary(provider))
				return
			}
			next.ServeHTTP(writer, request)
		})
	}
}

func clientIP(request *stdhttp.Request) string {
	address := strings.TrimSpace(request.RemoteAddr)
	if address == "" {
		return ""
	}

	if idx := strings.LastIndex(address, ":"); idx > 0 {
		host := address[:idx]
		host = strings.TrimPrefix(host, "[")
		host = strings.TrimSuffix(host, "]")
		if host != "" {
			return host
		}
	}
	return address
}

func writeRateLimitedResponse(writer stdhttp.ResponseWriter, requestID string, provider ProviderSummary) {
	writeMiddlewareErrorResponse(writer, requestID, stdhttp.StatusTooManyRequests, "1", ErrorResponse{
		Code:    "rate_limited",
		Message: "request rate exceeded; retry shortly",
	}, provider)
}
