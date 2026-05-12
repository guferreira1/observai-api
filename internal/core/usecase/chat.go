package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/core/ports"
)

// Chat answers follow-up questions about an active analysis.
type Chat struct {
	repository ports.AnalysisRepository
	cache      ports.AnalysisContextCache
	cacheTTL   time.Duration
	history    ports.ChatHistoryRepository
	responder  ports.ChatResponder
	scope      chatScopePolicy
	now        func() time.Time
}

// NewChat creates a scoped analysis chat use case.
func NewChat(
	repository ports.AnalysisRepository,
	cache ports.AnalysisContextCache,
	cacheTTL time.Duration,
	history ports.ChatHistoryRepository,
	responder ports.ChatResponder,
) *Chat {
	if cache == nil {
		cache = noOpAnalysisContextCache{}
	}
	if history == nil {
		history = noOpChatHistoryRepository{}
	}

	return &Chat{
		repository: repository,
		cache:      cache,
		cacheTTL:   cacheTTL,
		history:    history,
		responder:  responder,
		scope:      defaultChatScopePolicy(),
		now:        time.Now,
	}
}

// Ask answers a question only when it is related to the active analysis.
func (useCase *Chat) Ask(ctx context.Context, question domain.ChatQuestion) (domain.ChatAnswer, error) {
	if strings.TrimSpace(question.AnalysisID) == "" {
		return domain.ChatAnswer{}, fmt.Errorf("%w: analysis id is required", domain.ErrAnalysisNotFound)
	}

	if !useCase.scope.Allows(question.Question) {
		return domain.ChatAnswer{}, domain.ErrQuestionOutOfScope
	}

	analysisContext, err := useCase.findAnalysisContext(ctx, question.AnalysisID)
	if err != nil {
		return domain.ChatAnswer{}, err
	}

	answer, err := useCase.responder.Answer(ctx, analysisContext, question)
	if err != nil {
		return domain.ChatAnswer{}, fmt.Errorf("answer analysis question: %w", err)
	}

	err = useCase.history.SaveExchange(ctx, domain.ChatMessage{
		AnalysisID: question.AnalysisID,
		Role:       domain.ChatRoleUser,
		Content:    question.Question,
		CreatedAt:  useCase.now().UTC(),
	}, domain.ChatMessage{
		AnalysisID: answer.AnalysisID,
		Role:       domain.ChatRoleAssistant,
		Content:    answer.Answer,
		Evidence:   answer.Evidence,
		CreatedAt:  useCase.now().UTC(),
	})
	if err != nil {
		return domain.ChatAnswer{}, fmt.Errorf("save chat history: %w", err)
	}

	_ = useCase.cache.Save(ctx, analysisContext, useCase.cacheTTL)

	return answer, nil
}

// History returns persisted chat history for an analysis.
func (useCase *Chat) History(ctx context.Context, analysisID string) ([]domain.ChatMessage, error) {
	analysisID = strings.TrimSpace(analysisID)
	if analysisID == "" {
		return nil, fmt.Errorf("%w: analysis id is required", domain.ErrAnalysisNotFound)
	}

	if _, err := useCase.repository.Find(ctx, analysisID); err != nil {
		return nil, fmt.Errorf("find analysis: %w", err)
	}

	messages, err := useCase.history.List(ctx, analysisID)
	if err != nil {
		return nil, fmt.Errorf("list chat history: %w", err)
	}

	return messages, nil
}

func (useCase *Chat) findAnalysisContext(ctx context.Context, analysisID string) (domain.AnalysisContext, error) {
	analysisContext, err := useCase.cache.Find(ctx, analysisID)
	if err == nil {
		return analysisContext, nil
	}

	analysis, err := useCase.repository.Find(ctx, analysisID)
	if err != nil {
		return domain.AnalysisContext{}, fmt.Errorf("find analysis: %w", err)
	}

	analysisContext = domain.NewAnalysisContext(analysis)
	_ = useCase.cache.Save(ctx, analysisContext, useCase.cacheTTL)

	return analysisContext, nil
}
