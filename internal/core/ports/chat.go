package ports

import (
	"context"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// ChatHistoryRepository stores and retrieves persistent analysis chat history.
type ChatHistoryRepository interface {
	SaveExchange(ctx context.Context, question domain.ChatMessage, answer domain.ChatMessage) error
	List(ctx context.Context, analysisID string) ([]domain.ChatMessage, error)
}
