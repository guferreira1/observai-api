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

// WebhookRepository persists webhook subscriptions in PostgreSQL.
type WebhookRepository struct {
	pool     *pgxpool.Pool
	queries  *sqlc.Queries
	observer observability.ProviderObserver
}

// NewWebhookRepository builds a WebhookRepository sharing the pool.
func NewWebhookRepository(pool *pgxpool.Pool, opts ...RepositoryOptions) *WebhookRepository {
	observer := observability.ProviderObserver(observability.NoopProviderObserver{})
	if len(opts) > 0 && opts[0].Observer != nil {
		observer = opts[0].Observer
	}
	return &WebhookRepository{pool: pool, queries: sqlc.New(pool), observer: observer}
}

func (repository *WebhookRepository) observe(operation string, startedAt time.Time, err error) {
	repository.observer.Observe("postgres", operation, time.Since(startedAt), err)
}

// Create persists a new webhook subscription.
func (repository *WebhookRepository) Create(ctx context.Context, webhook domain.Webhook) (err error) {
	startedAt := time.Now()
	defer func() { repository.observe("create_webhook", startedAt, err) }()

	return repository.queries.CreateWebhookSubscription(ctx, sqlc.CreateWebhookSubscriptionParams{
		ID:        webhook.ID,
		Name:      webhook.Name,
		Url:       webhook.URL,
		Secret:    webhook.Secret,
		Event:     webhook.Event,
		CreatedAt: pgtype.Timestamptz{Time: webhook.CreatedAt, Valid: !webhook.CreatedAt.IsZero()},
	})
}

// Find returns the webhook subscription matching the supplied identifier.
func (repository *WebhookRepository) Find(ctx context.Context, id string) (webhook domain.Webhook, err error) {
	startedAt := time.Now()
	defer func() { repository.observe("find_webhook", startedAt, err) }()

	row, err := repository.queries.FindWebhookSubscription(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Webhook{}, domain.ErrWebhookNotFound
	}
	if err != nil {
		return domain.Webhook{}, fmt.Errorf("find webhook: %w", err)
	}
	return toDomainWebhook(row.ID, row.Name, row.Url, row.Secret, row.Event, row.CreatedAt, row.DisabledAt), nil
}

// List returns persisted webhooks newest first.
func (repository *WebhookRepository) List(ctx context.Context, limit int, offset int) (webhooks []domain.Webhook, err error) {
	startedAt := time.Now()
	defer func() { repository.observe("list_webhooks", startedAt, err) }()

	if limit <= 0 {
		limit = 50
	}
	rows, err := repository.queries.ListWebhookSubscriptions(ctx, sqlc.ListWebhookSubscriptionsParams{
		ResultLimit:  int32(limit),
		ResultOffset: int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("list webhooks: %w", err)
	}
	out := make([]domain.Webhook, 0, len(rows))
	for _, row := range rows {
		out = append(out, toDomainWebhook(row.ID, row.Name, row.Url, row.Secret, row.Event, row.CreatedAt, row.DisabledAt))
	}
	return out, nil
}

// ListActive returns webhooks subscribed to the supplied event.
func (repository *WebhookRepository) ListActive(ctx context.Context, event string) (webhooks []domain.Webhook, err error) {
	startedAt := time.Now()
	defer func() { repository.observe("list_active_webhooks", startedAt, err) }()

	rows, err := repository.queries.ListActiveWebhooksForEvent(ctx, event)
	if err != nil {
		return nil, fmt.Errorf("list active webhooks: %w", err)
	}
	out := make([]domain.Webhook, 0, len(rows))
	for _, row := range rows {
		out = append(out, toDomainWebhook(row.ID, row.Name, row.Url, row.Secret, row.Event, row.CreatedAt, row.DisabledAt))
	}
	return out, nil
}

// Update replaces mutable webhook subscription fields.
func (repository *WebhookRepository) Update(ctx context.Context, webhook domain.Webhook) (err error) {
	startedAt := time.Now()
	defer func() { repository.observe("update_webhook", startedAt, err) }()

	_, err = repository.queries.UpdateWebhookSubscription(ctx, sqlc.UpdateWebhookSubscriptionParams{
		ID:     webhook.ID,
		Name:   webhook.Name,
		Url:    webhook.URL,
		Secret: webhook.Secret,
		Event:  webhook.Event,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrWebhookNotFound
	}
	if err != nil {
		return fmt.Errorf("update webhook: %w", err)
	}
	return nil
}

// Disable marks a webhook subscription as disabled.
func (repository *WebhookRepository) Disable(ctx context.Context, id string) (err error) {
	startedAt := time.Now()
	defer func() { repository.observe("disable_webhook", startedAt, err) }()

	return repository.queries.DisableWebhookSubscription(ctx, sqlc.DisableWebhookSubscriptionParams{
		ID:         id,
		DisabledAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	})
}

func toDomainWebhook(id, name, url, secret, event string, createdAt, disabledAt pgtype.Timestamptz) domain.Webhook {
	webhook := domain.Webhook{
		ID:     id,
		Name:   name,
		URL:    url,
		Secret: secret,
		Event:  event,
	}
	if createdAt.Valid {
		webhook.CreatedAt = createdAt.Time.UTC()
	}
	if disabledAt.Valid {
		stamp := disabledAt.Time.UTC()
		webhook.DisabledAt = &stamp
	}
	return webhook
}
