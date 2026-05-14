package postgres

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

func TestAnalysisRepositoryIntegrationSaveAndFind(t *testing.T) {
	t.Parallel()

	repository := newIntegrationRepository(t)
	ctx := context.Background()
	result := domain.AnalysisResult{
		ID:               "test-analysis-repository-save-and-find",
		Summary:          "checkout-service latency increased",
		Severity:         domain.SeverityHigh,
		Confidence:       domain.ConfidenceMedium,
		AffectedServices: []string{"checkout-service", "payment-service"},
		Evidence: []domain.Evidence{
			{
				Signal:    domain.SignalMetrics,
				Service:   "checkout-service",
				Source:    "prometheus",
				Name:      "http_request_duration_seconds_p95",
				Summary:   "p95 latency exceeded baseline",
				Observed:  time.Date(2026, 5, 12, 10, 30, 0, 0, time.UTC),
				Score:     0.92,
				Unit:      "seconds",
				Reference: "promql:checkout:p95",
			},
		},
		DetectedAnomalies: []string{
			"p95 latency increased by 320%",
		},
		PossibleRootCauses: []domain.RootCauseHypothesis{
			{
				Cause:      "payment provider degradation",
				Evidence:   []string{"payment authorization span became dominant"},
				Confidence: domain.ConfidenceMedium,
			},
		},
		RecommendedActions: []domain.Recommendation{
			{
				Action:    "inspect payment provider latency",
				Rationale: "external dependency latency is correlated with checkout latency",
				Priority:  1,
			},
		},
		CodeLevelInsights: []string{
			"review timeout and fallback around payment authorization",
		},
		MissingEvidence: []string{
			"deployment timeline",
		},
		CreatedAt: time.Date(2026, 5, 12, 10, 45, 0, 0, time.UTC),
	}

	t.Cleanup(func() {
		_, err := repository.pool.Exec(context.Background(), "DELETE FROM analyses WHERE id = $1", result.ID)
		require.NoError(t, err)
	})

	require.NoError(t, repository.Save(ctx, result))

	stored, err := repository.Find(ctx, result.ID)
	require.NoError(t, err)

	assert.Equal(t, result.ID, stored.ID)
	assert.Equal(t, result.Summary, stored.Summary)
	assert.Equal(t, result.Severity, stored.Severity)
	assert.Equal(t, result.Confidence, stored.Confidence)
	assert.Equal(t, result.AffectedServices, stored.AffectedServices)
	assert.Equal(t, result.Evidence, stored.Evidence)
	assert.Equal(t, result.DetectedAnomalies, stored.DetectedAnomalies)
	assert.Equal(t, result.PossibleRootCauses, stored.PossibleRootCauses)
	assert.Equal(t, result.RecommendedActions, stored.RecommendedActions)
	assert.Equal(t, result.CodeLevelInsights, stored.CodeLevelInsights)
	assert.Equal(t, result.MissingEvidence, stored.MissingEvidence)
	assert.Equal(t, result.CreatedAt, stored.CreatedAt)
}

func TestAnalysisRepositoryIntegrationFindReturnsNotFound(t *testing.T) {
	t.Parallel()

	repository := newIntegrationRepository(t)

	_, err := repository.Find(context.Background(), "test-analysis-repository-missing")
	assert.True(t, errors.Is(err, domain.ErrAnalysisNotFound))
}

func TestAnalysisRepositoryIntegrationListAnalyses(t *testing.T) {
	t.Parallel()

	repository := newIntegrationRepository(t)
	ctx := context.Background()
	oldAnalysisID := "test-analysis-repository-list-old"
	newAnalysisID := "test-analysis-repository-list-new"

	require.NoError(t, repository.Save(ctx, domain.AnalysisResult{
		ID:               oldAnalysisID,
		Summary:          "old checkout analysis",
		Severity:         domain.SeverityMedium,
		Confidence:       domain.ConfidenceMedium,
		AffectedServices: []string{"checkout-service"},
		CreatedAt:        time.Date(2026, 5, 12, 9, 0, 0, 0, time.UTC),
	}))
	require.NoError(t, repository.Save(ctx, domain.AnalysisResult{
		ID:               newAnalysisID,
		Summary:          "new checkout analysis",
		Severity:         domain.SeverityHigh,
		Confidence:       domain.ConfidenceHigh,
		AffectedServices: []string{"checkout-service"},
		CreatedAt:        time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC),
	}))

	t.Cleanup(func() {
		_, err := repository.pool.Exec(context.Background(), "DELETE FROM analyses WHERE id = ANY($1)", []string{oldAnalysisID, newAnalysisID})
		require.NoError(t, err)
	})

	result, err := repository.ListAnalyses(ctx, domain.AnalysisListFilter{
		Limit:   1,
		Service: "checkout-service",
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.Items)
	assert.Equal(t, newAnalysisID, result.Items[0].ID)
	assert.GreaterOrEqual(t, result.Total, 2)
	assert.Equal(t, 1, result.Limit)
	assert.Equal(t, 0, result.Offset)
}

func TestAnalysisRepositoryIntegrationSaveExchangeAndList(t *testing.T) {
	t.Parallel()

	repository := newIntegrationRepository(t)
	ctx := context.Background()
	analysisID := "test-analysis-repository-chat-history"

	require.NoError(t, repository.Save(ctx, domain.AnalysisResult{
		ID:         analysisID,
		Summary:    "checkout-service latency increased",
		Severity:   domain.SeverityHigh,
		Confidence: domain.ConfidenceMedium,
		CreatedAt:  time.Date(2026, 5, 12, 10, 45, 0, 0, time.UTC),
	}))

	t.Cleanup(func() {
		_, err := repository.pool.Exec(context.Background(), "DELETE FROM analyses WHERE id = $1", analysisID)
		require.NoError(t, err)
	})

	require.NoError(t, repository.SaveExchange(ctx, domain.ChatMessage{
		AnalysisID: analysisID,
		Role:       domain.ChatRoleUser,
		Content:    "Which evidence supports this analysis?",
	}, domain.ChatMessage{
		AnalysisID: analysisID,
		Role:       domain.ChatRoleAssistant,
		Content:    "The latency metric supports this analysis.",
		Evidence:   []string{"p95_latency"},
	}))

	messages, err := repository.List(ctx, analysisID, domain.ChatHistoryFilter{})
	require.NoError(t, err)
	require.Len(t, messages, 2)
	assert.Equal(t, domain.ChatRoleUser, messages[0].Role)
	assert.Equal(t, "Which evidence supports this analysis?", messages[0].Content)
	assert.Equal(t, domain.ChatRoleAssistant, messages[1].Role)
	assert.Equal(t, []string{"p95_latency"}, messages[1].Evidence)
}

func TestAnalysisRepositoryIntegrationListAnalysesWithExtendedFilters(t *testing.T) {
	t.Parallel()

	repository := newIntegrationRepository(t)
	ctx := context.Background()
	prefix := "test-analysis-repo-extfilter-"
	ids := []string{prefix + "metric", prefix + "log", prefix + "trace"}
	scopedService := "extfilter-checkout"
	scopedGateway := "extfilter-payments"

	t.Cleanup(func() {
		_, err := repository.pool.Exec(context.Background(), "DELETE FROM analyses WHERE id = ANY($1)", ids)
		require.NoError(t, err)
	})

	base := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	require.NoError(t, repository.Save(ctx, domain.AnalysisResult{
		ID:               ids[0],
		Summary:          "metrics-driven analysis",
		Severity:         domain.SeverityMedium,
		Confidence:       domain.ConfidenceMedium,
		AffectedServices: []string{scopedService},
		Evidence: []domain.Evidence{
			{ID: "ev_1", Signal: domain.SignalMetrics, Service: scopedService, Provider: "extfilter-prometheus", Name: "p95"},
		},
		CreatedAt: base,
	}))
	require.NoError(t, repository.Save(ctx, domain.AnalysisResult{
		ID:               ids[1],
		Summary:          "log spike investigation",
		Severity:         domain.SeverityHigh,
		Confidence:       domain.ConfidenceHigh,
		AffectedServices: []string{scopedService},
		Evidence: []domain.Evidence{
			{ID: "ev_1", Signal: domain.SignalLogs, Service: scopedService, Provider: "extfilter-loki", Name: "5xx_rate"},
		},
		CreatedAt: base.Add(time.Hour),
	}))
	require.NoError(t, repository.Save(ctx, domain.AnalysisResult{
		ID:               ids[2],
		Summary:          "trace-only extfilter-payment latency",
		Severity:         domain.SeverityLow,
		Confidence:       domain.ConfidenceLow,
		AffectedServices: []string{scopedGateway},
		Evidence: []domain.Evidence{
			{ID: "ev_1", Signal: domain.SignalTraces, Service: scopedGateway, Provider: "extfilter-jaeger", Name: "trace"},
		},
		CreatedAt: base.Add(2 * time.Hour),
	}))

	bySignal, err := repository.ListAnalyses(ctx, domain.AnalysisListFilter{Limit: 10, Service: scopedService, Signal: domain.SignalLogs})
	require.NoError(t, err)
	assert.Equal(t, 1, bySignal.Total)
	require.Len(t, bySignal.Items, 1)
	assert.Equal(t, ids[1], bySignal.Items[0].ID)

	byProvider, err := repository.ListAnalyses(ctx, domain.AnalysisListFilter{Limit: 10, Provider: "extfilter-jaeger"})
	require.NoError(t, err)
	assert.Equal(t, 1, byProvider.Total)
	assert.Equal(t, ids[2], byProvider.Items[0].ID)

	byQuery, err := repository.ListAnalyses(ctx, domain.AnalysisListFilter{Limit: 10, Query: "extfilter-payment"})
	require.NoError(t, err)
	assert.Equal(t, 1, byQuery.Total)
	assert.Equal(t, ids[2], byQuery.Items[0].ID)

	bySeverityAsc, err := repository.ListAnalyses(ctx, domain.AnalysisListFilter{
		Limit:   10,
		Service: scopedService,
		Sort:    domain.SortBySeverity,
		Order:   domain.OrderAsc,
	})
	require.NoError(t, err)
	require.Len(t, bySeverityAsc.Items, 2)
	assert.Equal(t, ids[0], bySeverityAsc.Items[0].ID)
	assert.Equal(t, ids[1], bySeverityAsc.Items[1].ID)

	byTimeWindow, err := repository.ListAnalyses(ctx, domain.AnalysisListFilter{
		Limit:   10,
		Service: scopedService,
		From:    base.Add(30 * time.Minute),
		To:      base.Add(90 * time.Minute),
	})
	require.NoError(t, err)
	assert.Equal(t, 1, byTimeWindow.Total)
	assert.Equal(t, ids[1], byTimeWindow.Items[0].ID)
}

func TestAnalysisRepositoryIntegrationStatsAndServices(t *testing.T) {
	t.Parallel()

	repository := newIntegrationRepository(t)
	ctx := context.Background()
	prefix := "test-analysis-repo-stats-"
	ids := []string{prefix + "a", prefix + "b"}
	scopedService := "stats-checkout"
	scopedGateway := "stats-payments"

	t.Cleanup(func() {
		_, err := repository.pool.Exec(context.Background(), "DELETE FROM analyses WHERE id = ANY($1)", ids)
		require.NoError(t, err)
	})

	base := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	require.NoError(t, repository.Save(ctx, domain.AnalysisResult{
		ID:               ids[0],
		Summary:          "stats sample a",
		Severity:         domain.SeverityHigh,
		Confidence:       domain.ConfidenceHigh,
		AffectedServices: []string{scopedService, scopedGateway},
		CreatedAt:        base,
	}))
	require.NoError(t, repository.Save(ctx, domain.AnalysisResult{
		ID:               ids[1],
		Summary:          "stats sample b",
		Severity:         domain.SeverityMedium,
		Confidence:       domain.ConfidenceMedium,
		AffectedServices: []string{scopedService},
		CreatedAt:        base.Add(time.Hour),
	}))

	stats, err := repository.AnalysisStats(ctx, domain.AnalysisStatsFilter{
		Service: scopedService,
		From:    base.Add(-time.Hour),
		To:      base.Add(2 * time.Hour),
	}, 5)
	require.NoError(t, err)
	assert.Equal(t, 2, stats.Total)
	assert.Equal(t, 1, stats.BySeverity[domain.SeverityHigh])
	assert.Equal(t, 1, stats.BySeverity[domain.SeverityMedium])
	assert.Equal(t, 1, stats.ByConfidence[domain.ConfidenceHigh])
	assert.NotEmpty(t, stats.TopAffectedServices)
	assert.NotEmpty(t, stats.TrendBuckets)

	services, err := repository.ListAffectedServices(ctx, "stats-check", 10)
	require.NoError(t, err)
	assert.Contains(t, services, scopedService)
}

func newIntegrationRepository(t *testing.T) *AnalysisRepository {
	t.Helper()

	dsn := os.Getenv("OBSERVAI_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("OBSERVAI_TEST_DATABASE_DSN is required for PostgreSQL integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repository, err := NewAnalysisRepository(ctx, dsn)
	require.NoError(t, err)

	t.Cleanup(repository.Close)

	return repository
}
