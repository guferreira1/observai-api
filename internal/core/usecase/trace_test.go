package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/guferreira1/observai-api/internal/adapters/outbound/inmemory"
	"github.com/guferreira1/observai-api/internal/adapters/outbound/testfakes"
	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTraceGetReturnsTraceNotFoundWhenAnalysisHasNoTraceID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repository := inmemory.NewAnalysisRepository()
	require.NoError(t, repository.Save(ctx, domain.AnalysisResult{ID: "analysis-1"}))

	useCase := NewTrace(repository, testfakes.NewTraceProvider())
	_, err := useCase.Get(ctx, "analysis-1")

	assert.True(t, errors.Is(err, domain.ErrTraceNotFound))
}

func TestTraceGetUsesStoredTraceID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repository := inmemory.NewAnalysisRepository()
	require.NoError(t, repository.Save(ctx, domain.AnalysisResult{
		ID:      "analysis-1",
		TraceID: "trace-1",
	}))

	useCase := NewTrace(repository, testfakes.NewTraceProvider())
	insights, err := useCase.Get(ctx, "analysis-1")

	require.NoError(t, err)
	require.NotEmpty(t, insights.Spans)
	assert.Equal(t, "trace-1", insights.Spans[0].TraceID)
}
