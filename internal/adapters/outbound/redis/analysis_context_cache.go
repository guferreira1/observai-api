package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
	redisclient "github.com/redis/go-redis/v9"
)

const analysisContextKeyPrefix = "observai:analysis-context:v1:"

// AnalysisContextCache stores compact analysis contexts in Redis.
type AnalysisContextCache struct {
	client *redisclient.Client
}

// NewAnalysisContextCache creates a Redis-backed analysis context cache.
func NewAnalysisContextCache(ctx context.Context, redisURL string) (*AnalysisContextCache, error) {
	options, err := redisclient.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}

	client := redisclient.NewClient(options)
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	return &AnalysisContextCache{client: client}, nil
}

// Close releases Redis connections held by the cache.
func (cache *AnalysisContextCache) Close() error {
	if err := cache.client.Close(); err != nil {
		return fmt.Errorf("close redis client: %w", err)
	}

	return nil
}

// Save stores an analysis context until the provided TTL expires.
func (cache *AnalysisContextCache) Save(ctx context.Context, analysisContext domain.AnalysisContext, ttl time.Duration) error {
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
func (cache *AnalysisContextCache) Find(ctx context.Context, analysisID string) (domain.AnalysisContext, error) {
	payload, err := cache.client.Get(ctx, analysisContextKey(analysisID)).Bytes()
	if errors.Is(err, redisclient.Nil) {
		return domain.AnalysisContext{}, fmt.Errorf("%w: %s", domain.ErrAnalysisContextNotFound, analysisID)
	}
	if err != nil {
		return domain.AnalysisContext{}, fmt.Errorf("get analysis context cache: %w", err)
	}

	var analysisContext domain.AnalysisContext
	if err := json.Unmarshal(payload, &analysisContext); err != nil {
		return domain.AnalysisContext{}, fmt.Errorf("unmarshal analysis context: %w", err)
	}

	return analysisContext, nil
}

func analysisContextKey(analysisID string) string {
	return analysisContextKeyPrefix + analysisID
}
