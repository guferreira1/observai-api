package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/guferreira1/observai-api/internal/adapters/outbound/fake"
	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnalysisAnalyze(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repository := fake.NewAnalysisRepository()
	useCase := NewAnalysis(
		fake.NewSignalCollector(),
		fake.NewAnalysisGenerator(),
		repository,
		fake.NewAnalysisContextCache(),
		6*time.Hour,
		fake.NewIDGenerator("analysis"),
	)

	result, err := useCase.Analyze(ctx, domain.AnalysisRequest{
		Goal: "investigate checkout latency",
		TimeWindow: domain.TimeWindow{
			Start: time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC),
			End:   time.Date(2026, 5, 12, 11, 0, 0, 0, time.UTC),
		},
		AffectedServices: []string{"checkout-service"},
		Signals:          []domain.SignalType{domain.SignalLogs, domain.SignalMetrics, domain.SignalTraces},
	})
	require.NoError(t, err)
	assert.Equal(t, "analysis-000001", result.ID)
	assert.Equal(t, domain.SeverityHigh, result.Severity)
	assert.Len(t, result.Evidence, 3)

	stored, err := repository.Find(ctx, result.ID)
	require.NoError(t, err)
	assert.Equal(t, result.Summary, stored.Summary)
}

func TestAnalysisAnalyzeRejectsInvalidRequest(t *testing.T) {
	t.Parallel()

	useCase := NewAnalysis(
		fake.NewSignalCollector(),
		fake.NewAnalysisGenerator(),
		fake.NewAnalysisRepository(),
		fake.NewAnalysisContextCache(),
		6*time.Hour,
		fake.NewIDGenerator("analysis"),
	)

	_, err := useCase.Analyze(context.Background(), domain.AnalysisRequest{})
	assert.True(t, errors.Is(err, domain.ErrInvalidAnalysisRequest))
}
