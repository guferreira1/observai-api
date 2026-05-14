package redis

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnalysisContextCacheIntegrationSaveAndFind(t *testing.T) {
	t.Parallel()

	cache := newIntegrationCache(t)
	ctx := context.Background()
	analysisContext := domain.AnalysisContext{
		AnalysisID:          "test-analysis-context-cache-save-and-find",
		Summary:             "checkout-service latency increased",
		Severity:            domain.SeverityHigh,
		Confidence:          domain.ConfidenceMedium,
		AffectedServices:    []string{"checkout-service"},
		Evidence:            []domain.Evidence{{Name: "p95_latency", Signal: domain.SignalMetrics}},
		DetectedAnomalies:   []string{"p95 latency increased"},
		PossibleRootCauses:  []domain.RootCauseHypothesis{{Cause: "payment provider degradation"}},
		RecommendedActions:  []domain.Recommendation{{Action: "inspect payment provider latency", Priority: 1}},
		CodeLevelInsights:   []string{"review timeout and fallback"},
		MissingEvidence:     []string{"deployment timeline"},
		AnalysisCompletedAt: time.Date(2026, 5, 12, 10, 45, 0, 0, time.UTC),
	}

	require.NoError(t, cache.Save(ctx, analysisContext, 5*time.Minute))
	t.Cleanup(func() {
		require.NoError(t, cache.client.Del(context.Background(), analysisContextKey(analysisContext.AnalysisID)).Err())
	})

	stored, err := cache.Find(ctx, analysisContext.AnalysisID)
	require.NoError(t, err)
	assert.Equal(t, analysisContext, stored)
}

func TestAnalysisContextCacheIntegrationFindReturnsNotFound(t *testing.T) {
	t.Parallel()

	cache := newIntegrationCache(t)

	_, err := cache.Find(context.Background(), "test-analysis-context-cache-missing")
	assert.True(t, errors.Is(err, domain.ErrAnalysisContextNotFound))
}

func newIntegrationCache(t *testing.T) *AnalysisContextCache {
	t.Helper()

	redisURL := os.Getenv("OBSERVAI_TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("OBSERVAI_TEST_REDIS_URL is required for Redis integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cache, err := NewAnalysisContextCache(ctx, redisURL)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, cache.Close())
	})

	return cache
}
