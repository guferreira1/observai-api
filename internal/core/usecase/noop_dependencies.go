package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

type noOpAnalysisContextCache struct{}

type noOpChatHistoryRepository struct{}

func (noOpAnalysisContextCache) Save(context.Context, domain.AnalysisContext, time.Duration) error {
	return nil
}

func (noOpAnalysisContextCache) Find(_ context.Context, analysisID string) (domain.AnalysisContext, error) {
	return domain.AnalysisContext{}, fmt.Errorf("%w: %s", domain.ErrAnalysisContextNotFound, analysisID)
}

func (noOpChatHistoryRepository) SaveExchange(context.Context, domain.ChatMessage, domain.ChatMessage) error {
	return nil
}

func (noOpChatHistoryRepository) List(context.Context, string) ([]domain.ChatMessage, error) {
	return []domain.ChatMessage{}, nil
}
