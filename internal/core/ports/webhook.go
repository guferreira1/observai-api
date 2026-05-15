package ports

import (
	"context"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// WebhookRepository persists webhook subscriptions and supports lookup by event.
type WebhookRepository interface {
	Create(ctx context.Context, webhook domain.Webhook) error
	Find(ctx context.Context, id string) (domain.Webhook, error)
	List(ctx context.Context, limit int, offset int) ([]domain.Webhook, error)
	ListActive(ctx context.Context, event string) ([]domain.Webhook, error)
	Disable(ctx context.Context, id string) error
}

// WebhookDispatcher delivers webhook payloads asynchronously.
//
// Implementations sign the payload with the webhook secret, POST it to
// the receiver and update the supplied delivery row via the configured
// WebhookDeliveryRepository when the attempt completes. The call returns
// immediately; delivery happens on a background goroutine.
type WebhookDispatcher interface {
	Dispatch(ctx context.Context, webhook domain.Webhook, delivery domain.WebhookDelivery) error
}

// WebhookDeliveryRepository persists webhook delivery attempts so operators
// can inspect history, retry failed deliveries and the scheduler can
// pick up pending entries on a cron tick.
type WebhookDeliveryRepository interface {
	Create(ctx context.Context, delivery domain.WebhookDelivery) error
	Find(ctx context.Context, id string) (domain.WebhookDelivery, error)
	List(ctx context.Context, limit int, offset int) ([]domain.WebhookDelivery, error)
	ListPending(ctx context.Context, cutoff time.Time, limit int) ([]domain.WebhookDelivery, error)
	MarkDelivered(ctx context.Context, id string, responseStatus int, when time.Time) error
	MarkFailed(ctx context.Context, id string, attempt int, responseStatus int, lastError string, when time.Time) error
	ScheduleRetry(ctx context.Context, id string, attempt int, responseStatus int, lastError string, nextAttemptAt time.Time, when time.Time) error
}
