package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/platform/observability"
	redisclient "github.com/redis/go-redis/v9"
)

const analysisContextKeyPrefix = "observai:analysis-context:v1:"

// AnalysisContextCache stores compact analysis contexts in Redis.
type AnalysisContextCache struct {
	client   *redisclient.Client
	observer observability.ProviderObserver
}

// CacheOptions configures optional collaborators for the analysis context cache.
type CacheOptions struct {
	Observer observability.ProviderObserver
}

// NewAnalysisContextCache creates a Redis-backed analysis context cache.
func NewAnalysisContextCache(ctx context.Context, redisURL string, opts ...CacheOptions) (*AnalysisContextCache, error) {
	options, err := redisclient.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}

	client := redisclient.NewClient(options)
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	observer := observability.ProviderObserver(observability.NoopProviderObserver{})
	if len(opts) > 0 && opts[0].Observer != nil {
		observer = opts[0].Observer
	}

	return &AnalysisContextCache{client: client, observer: observer}, nil
}

// Ping verifies connectivity to Redis with the supplied context.
func (cache *AnalysisContextCache) Ping(ctx context.Context) error {
	return cache.client.Ping(ctx).Err()
}

// Close releases Redis connections held by the cache.
func (cache *AnalysisContextCache) Close() error {
	if err := cache.client.Close(); err != nil {
		return fmt.Errorf("close redis client: %w", err)
	}

	return nil
}

// Save stores an analysis context until the provided TTL expires.
func (cache *AnalysisContextCache) Save(ctx context.Context, analysisContext domain.AnalysisContext, ttl time.Duration) (err error) {
	startedAt := time.Now()
	defer func() {
		cache.observer.Observe("redis", "save_analysis_context", time.Since(startedAt), err)
	}()

	payload, err := json.Marshal(analysisContext)
	if err != nil {
		return fmt.Errorf("marshal analysis context: %w", err)
	}

	if err := cache.client.Set(ctx, analysisContextKey(analysisContext.AnalysisID), payload, ttl).Err(); err != nil {
		return fmt.Errorf("set analysis context cache: %w", err)
	}

	return nil
}

// Find returns a cached analysis context by analysis identifier.
func (cache *AnalysisContextCache) Find(ctx context.Context, analysisID string) (analysisContext domain.AnalysisContext, err error) {
	startedAt := time.Now()
	defer func() {
		cache.observer.Observe("redis", "find_analysis_context", time.Since(startedAt), err)
	}()

	payload, err := cache.client.Get(ctx, analysisContextKey(analysisID)).Bytes()
	if errors.Is(err, redisclient.Nil) {
		return domain.AnalysisContext{}, fmt.Errorf("%w: %s", domain.ErrAnalysisContextNotFound, analysisID)
	}
	if err != nil {
		return domain.AnalysisContext{}, fmt.Errorf("get analysis context cache: %w", err)
	}

	if err := json.Unmarshal(payload, &analysisContext); err != nil {
		return domain.AnalysisContext{}, fmt.Errorf("unmarshal analysis context: %w", err)
	}

	return analysisContext, nil
}

func analysisContextKey(analysisID string) string {
	return analysisContextKeyPrefix + analysisID
}
