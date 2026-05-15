package ports

import (
	"context"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// ChatHistoryRepository stores and retrieves persistent analysis chat history.
//
// List returns the persisted exchange honoring the supplied pagination filter.
// Messages are returned oldest-first; when filter.Before is non-zero only
// messages strictly older than the cursor are returned.
type ChatHistoryRepository interface {
	SaveExchange(ctx context.Context, question domain.ChatMessage, answer domain.ChatMessage) error
	List(ctx context.Context, analysisID string, filter domain.ChatHistoryFilter) ([]domain.ChatMessage, error)
	FindMessage(ctx context.Context, id string) (domain.ChatMessage, error)
}

// ChatFeedbackRepository persists user feedback on assistant chat answers.
type ChatFeedbackRepository interface {
	SaveFeedback(ctx context.Context, feedback domain.ChatFeedback) error
}
