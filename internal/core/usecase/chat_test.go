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

func TestChatAskAnswersAnalysisRelatedQuestion(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repository := fake.NewAnalysisRepository()
	err := repository.Save(ctx, domain.AnalysisResult{
		ID:      "analysis-000001",
		Summary: "checkout-service latency increased",
		Evidence: []domain.Evidence{
			{Name: "p95_latency"},
		},
	})
	require.NoError(t, err)

	useCase := NewChat(repository, fake.NewAnalysisContextCache(), 6*time.Hour, repository, fake.NewChatResponder())

	answer, err := useCase.Ask(ctx, domain.ChatQuestion{
		AnalysisID: "analysis-000001",
		Question:   "Which evidence supports this analysis?",
	})
	require.NoError(t, err)
	assert.Equal(t, "analysis-000001", answer.AnalysisID)
	assert.Equal(t, []string{"p95_latency"}, answer.Evidence)

	messages, err := useCase.History(ctx, "analysis-000001")
	require.NoError(t, err)
	require.Len(t, messages, 2)
	assert.Equal(t, domain.ChatRoleUser, messages[0].Role)
	assert.Equal(t, domain.ChatRoleAssistant, messages[1].Role)
}

func TestChatAskRejectsOutOfScopeQuestion(t *testing.T) {
	t.Parallel()

	repository := fake.NewAnalysisRepository()
	useCase := NewChat(repository, fake.NewAnalysisContextCache(), 6*time.Hour, repository, fake.NewChatResponder())

	_, err := useCase.Ask(context.Background(), domain.ChatQuestion{
		AnalysisID: "analysis-000001",
		Question:   "What is the capital of France?",
	})
	assert.True(t, errors.Is(err, domain.ErrQuestionOutOfScope))
}

func TestChatAskReturnsNotFoundForMissingAnalysis(t *testing.T) {
	t.Parallel()

	repository := fake.NewAnalysisRepository()
	useCase := NewChat(repository, fake.NewAnalysisContextCache(), 6*time.Hour, repository, fake.NewChatResponder())

	_, err := useCase.Ask(context.Background(), domain.ChatQuestion{
		AnalysisID: "analysis-000001",
		Question:   "What caused this incident?",
	})
	assert.True(t, errors.Is(err, domain.ErrAnalysisNotFound))
}

func TestChatWorksWithoutOptionalCacheAndHistory(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repository := fake.NewAnalysisRepository()
	err := repository.Save(ctx, domain.AnalysisResult{
		ID:      "analysis-000001",
		Summary: "checkout-service latency increased",
		Evidence: []domain.Evidence{
			{Name: "p95_latency"},
		},
	})
	require.NoError(t, err)

	useCase := NewChat(repository, nil, 6*time.Hour, nil, fake.NewChatResponder())

	answer, err := useCase.Ask(ctx, domain.ChatQuestion{
		AnalysisID: "analysis-000001",
		Question:   "Which evidence supports this analysis?",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"p95_latency"}, answer.Evidence)

	messages, err := useCase.History(ctx, "analysis-000001")
	require.NoError(t, err)
	assert.Empty(t, messages)
}

func TestChatScopePolicy(t *testing.T) {
	t.Parallel()

	policy := defaultChatScopePolicy()
	tests := []struct {
		name     string
		question string
		allowed  bool
	}{
		{
			name:     "allows evidence question",
			question: "Which evidence supports this analysis?",
			allowed:  true,
		},
		{
			name:     "allows portuguese analysis question",
			question: "Qual métrica indica o gargalo?",
			allowed:  true,
		},
		{
			name:     "rejects unrelated question",
			question: "What is the capital of France?",
			allowed:  false,
		},
		{
			name:     "rejects blank question",
			question: "   ",
			allowed:  false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.allowed, policy.Allows(test.question))
		})
	}
}
