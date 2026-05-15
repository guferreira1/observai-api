package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/guferreira1/observai-api/internal/adapters/outbound/postgres/sqlc"
	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/platform/observability"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// WebhookDeliveryRepository persists webhook delivery attempts in PostgreSQL.
type WebhookDeliveryRepository struct {
	pool     *pgxpool.Pool
	queries  *sqlc.Queries
	observer observability.ProviderObserver
}

// NewWebhookDeliveryRepository creates a postgres-backed delivery repository.
func NewWebhookDeliveryRepository(pool *pgxpool.Pool, opts ...RepositoryOptions) *WebhookDeliveryRepository {
	observer := observability.ProviderObserver(observability.NoopProviderObserver{})
	if len(opts) > 0 && opts[0].Observer != nil {
		observer = opts[0].Observer
	}
	return &WebhookDeliveryRepository{pool: pool, queries: sqlc.New(pool), observer: observer}
}

func (repository *WebhookDeliveryRepository) observe(operation string, startedAt time.Time, err error) {
	repository.observer.Observe("postgres", operation, time.Since(startedAt), err)
}

// Create persists a new delivery row.
func (repository *WebhookDeliveryRepository) Create(ctx context.Context, delivery domain.WebhookDelivery) (err error) {
	startedAt := time.Now()
	defer func() { repository.observe("create_webhook_delivery", startedAt, err) }()

	params := sqlc.CreateWebhookDeliveryParams{
		ID:             delivery.ID,
		SubscriptionID: delivery.SubscriptionID,
		Event:          delivery.Event,
		Payload:        delivery.Payload,
		Status:         string(delivery.Status),
		Attempt:        int32(delivery.Attempt),
		CreatedAt:      pgtype.Timestamptz{Time: delivery.CreatedAt, Valid: !delivery.CreatedAt.IsZero()},
		UpdatedAt:      pgtype.Timestamptz{Time: delivery.UpdatedAt, Valid: !delivery.UpdatedAt.IsZero()},
	}
	if delivery.NextAttemptAt != nil {
		params.NextAttemptAt = pgtype.Timestamptz{Time: *delivery.NextAttemptAt, Valid: true}
	}
	return repository.queries.CreateWebhookDelivery(ctx, params)
}

// Find returns the delivery matching the supplied identifier.
func (repository *WebhookDeliveryRepository) Find(ctx context.Context, id string) (delivery domain.WebhookDelivery, err error) {
	startedAt := time.Now()
	defer func() { repository.observe("find_webhook_delivery", startedAt, err) }()

	row, err := repository.queries.FindWebhookDelivery(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.WebhookDelivery{}, domain.ErrWebhookDeliveryNotFound
	}
	if err != nil {
		return domain.WebhookDelivery{}, fmt.Errorf("find webhook delivery: %w", err)
	}
	return rowToWebhookDelivery(row.ID, row.SubscriptionID, row.Event, row.Payload, row.Status, row.Attempt, row.LastError, row.ResponseStatus, row.NextAttemptAt, row.DeliveredAt, row.CreatedAt, row.UpdatedAt), nil
}

// List returns persisted deliveries newest first.
func (repository *WebhookDeliveryRepository) List(ctx context.Context, limit int, offset int) (deliveries []domain.WebhookDelivery, err error) {
	startedAt := time.Now()
	defer func() { repository.observe("list_webhook_deliveries", startedAt, err) }()

	rows, err := repository.queries.ListWebhookDeliveries(ctx, sqlc.ListWebhookDeliveriesParams{
		ResultLimit:  int32(limit),
		ResultOffset: int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("list webhook deliveries: %w", err)
	}
	out := make([]domain.WebhookDelivery, 0, len(rows))
	for _, row := range rows {
		out = append(out, rowToWebhookDelivery(row.ID, row.SubscriptionID, row.Event, row.Payload, row.Status, row.Attempt, row.LastError, row.ResponseStatus, row.NextAttemptAt, row.DeliveredAt, row.CreatedAt, row.UpdatedAt))
	}
	return out, nil
}

// ListPending returns deliveries with status='pending' due before cutoff.
func (repository *WebhookDeliveryRepository) ListPending(ctx context.Context, cutoff time.Time, limit int) (deliveries []domain.WebhookDelivery, err error) {
	startedAt := time.Now()
	defer func() { repository.observe("list_pending_webhook_deliveries", startedAt, err) }()

	if limit <= 0 {
		limit = 50
	}
	rows, err := repository.queries.ListPendingWebhookDeliveries(ctx, sqlc.ListPendingWebhookDeliveriesParams{
		Cutoff:      pgtype.Timestamptz{Time: cutoff, Valid: true},
		ResultLimit: int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list pending webhook deliveries: %w", err)
	}
	out := make([]domain.WebhookDelivery, 0, len(rows))
	for _, row := range rows {
		out = append(out, rowToWebhookDelivery(row.ID, row.SubscriptionID, row.Event, row.Payload, row.Status, row.Attempt, row.LastError, row.ResponseStatus, row.NextAttemptAt, row.DeliveredAt, row.CreatedAt, row.UpdatedAt))
	}
	return out, nil
}

// MarkDelivered records a successful attempt.
func (repository *WebhookDeliveryRepository) MarkDelivered(ctx context.Context, id string, responseStatus int, when time.Time) (err error) {
	startedAt := time.Now()
	defer func() { repository.observe("mark_webhook_delivered", startedAt, err) }()
	return repository.queries.MarkWebhookDeliveryDelivered(ctx, sqlc.MarkWebhookDeliveryDeliveredParams{
		ID:             id,
		ResponseStatus: pgtype.Int4{Int32: int32(responseStatus), Valid: true},
		DeliveredAt:    pgtype.Timestamptz{Time: when, Valid: true},
		UpdatedAt:      pgtype.Timestamptz{Time: when, Valid: true},
	})
}

// MarkFailed flags the delivery as permanently failed.
func (repository *WebhookDeliveryRepository) MarkFailed(ctx context.Context, id string, attempt int, responseStatus int, lastError string, when time.Time) (err error) {
	startedAt := time.Now()
	defer func() { repository.observe("mark_webhook_failed", startedAt, err) }()
	return repository.queries.MarkWebhookDeliveryFailed(ctx, sqlc.MarkWebhookDeliveryFailedParams{
		ID:             id,
		Attempt:        int32(attempt),
		LastError:      pgtype.Text{String: lastError, Valid: lastError != ""},
		ResponseStatus: pgtype.Int4{Int32: int32(responseStatus), Valid: responseStatus > 0},
		UpdatedAt:      pgtype.Timestamptz{Time: when, Valid: true},
	})
}

// ScheduleRetry re-queues the delivery for a future attempt.
func (repository *WebhookDeliveryRepository) ScheduleRetry(ctx context.Context, id string, attempt int, responseStatus int, lastError string, nextAttemptAt time.Time, when time.Time) (err error) {
	startedAt := time.Now()
	defer func() { repository.observe("schedule_webhook_retry", startedAt, err) }()
	return repository.queries.ScheduleWebhookDeliveryRetry(ctx, sqlc.ScheduleWebhookDeliveryRetryParams{
		ID:             id,
		Attempt:        int32(attempt),
		LastError:      pgtype.Text{String: lastError, Valid: lastError != ""},
		ResponseStatus: pgtype.Int4{Int32: int32(responseStatus), Valid: responseStatus > 0},
		NextAttemptAt:  pgtype.Timestamptz{Time: nextAttemptAt, Valid: true},
		UpdatedAt:      pgtype.Timestamptz{Time: when, Valid: true},
	})
}

func rowToWebhookDelivery(id, subscriptionID, event string, payload []byte, status string, attempt int32, lastError pgtype.Text, responseStatus pgtype.Int4, nextAttemptAt, deliveredAt, createdAt, updatedAt pgtype.Timestamptz) domain.WebhookDelivery {
	delivery := domain.WebhookDelivery{
		ID:             id,
		SubscriptionID: subscriptionID,
		Event:          event,
		Payload:        payload,
		Status:         domain.WebhookDeliveryStatus(status),
		Attempt:        int(attempt),
	}
	if lastError.Valid {
		delivery.LastError = lastError.String
	}
	if responseStatus.Valid {
		delivery.ResponseStatus = int(responseStatus.Int32)
	}
	if nextAttemptAt.Valid {
		next := nextAttemptAt.Time.UTC()
		delivery.NextAttemptAt = &next
	}
	if deliveredAt.Valid {
		delivered := deliveredAt.Time.UTC()
		delivery.DeliveredAt = &delivered
	}
	if createdAt.Valid {
		delivery.CreatedAt = createdAt.Time.UTC()
	}
	if updatedAt.Valid {
		delivery.UpdatedAt = updatedAt.Time.UTC()
	}
	return delivery
}
