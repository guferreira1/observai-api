package ports

import (
	"context"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// WebhookRepository persists webhook subscriptions and supports lookup by event.
type WebhookRepository interface {
	Create(ctx context.Context, webhook domain.Webhook) error
	List(ctx context.Context, limit int, offset int) ([]domain.Webhook, error)
	ListActive(ctx context.Context, event string) ([]domain.Webhook, error)
	Disable(ctx context.Context, id string) error
}

// WebhookDispatcher delivers webhook payloads asynchronously.
//
// Implementations are responsible for retries and signing; the use case
// only enqueues a delivery and returns immediately.
type WebhookDispatcher interface {
	Dispatch(ctx context.Context, webhook domain.Webhook, event string, payload []byte) error
}
