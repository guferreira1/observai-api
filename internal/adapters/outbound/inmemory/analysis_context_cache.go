package inmemory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// AnalysisContextCache stores analysis contexts in memory for deterministic tests.
type AnalysisContextCache struct {
	mu       sync.RWMutex
	contexts map[string]cachedAnalysisContext
	now      func() time.Time
}

type cachedAnalysisContext struct {
	context   domain.AnalysisContext
	expiresAt time.Time
}

// NewAnalysisContextCache creates an in-memory analysis context cache.
func NewAnalysisContextCache() *AnalysisContextCache {
	return &AnalysisContextCache{
		contexts: make(map[string]cachedAnalysisContext),
		now:      time.Now,
	}
}

// Save stores an analysis context until the provided TTL expires.
func (cache *AnalysisContextCache) Save(_ context.Context, analysisContext domain.AnalysisContext, ttl time.Duration) error {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	cache.contexts[analysisContext.AnalysisID] = cachedAnalysisContext{
		context:   analysisContext,
		expiresAt: cache.now().Add(ttl),
	}

	return nil
}

// Find returns a cached analysis context by analysis identifier.
func (cache *AnalysisContextCache) Find(_ context.Context, analysisID string) (domain.AnalysisContext, error) {
	cache.mu.RLock()
	item, ok := cache.contexts[analysisID]
	cache.mu.RUnlock()
	if !ok {
		return domain.AnalysisContext{}, fmt.Errorf("%w: %s", domain.ErrAnalysisContextNotFound, analysisID)
	}

	if cache.now().After(item.expiresAt) {
		cache.mu.Lock()
		delete(cache.contexts, analysisID)
		cache.mu.Unlock()
		return domain.AnalysisContext{}, fmt.Errorf("%w: %s", domain.ErrAnalysisContextNotFound, analysisID)
	}

	return item.context, nil
}
