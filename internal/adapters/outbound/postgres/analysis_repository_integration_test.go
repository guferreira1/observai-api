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

func TestAnalysisRepositoryIntegrationSaveExchangeAndList(t *testing.T) {
	t.Parallel()

	repository := newIntegrationRepository(t)
	ctx := context.Background()
	analysisID := "test-analysis-repository-chat-history"

	require.NoError(t, repository.Save(ctx, domain.AnalysisResult{
		ID:        analysisID,
		Summary:   "checkout-service latency increased",
		Severity:  domain.SeverityHigh,
		CreatedAt: time.Date(2026, 5, 12, 10, 45, 0, 0, time.UTC),
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

	messages, err := repository.List(ctx, analysisID)
	require.NoError(t, err)
	require.Len(t, messages, 2)
	assert.Equal(t, domain.ChatRoleUser, messages[0].Role)
	assert.Equal(t, "Which evidence supports this analysis?", messages[0].Content)
	assert.Equal(t, domain.ChatRoleAssistant, messages[1].Role)
	assert.Equal(t, []string{"p95_latency"}, messages[1].Evidence)
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
