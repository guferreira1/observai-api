package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/guferreira1/observai-api/internal/adapters/outbound/postgres/sqlc"
	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/platform/observability"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AuditLogRepository persists audit_log rows in PostgreSQL.
type AuditLogRepository struct {
	pool     *pgxpool.Pool
	queries  *sqlc.Queries
	observer observability.ProviderObserver
}

// NewAuditLogRepository builds an AuditLogRepository sharing the pool.
func NewAuditLogRepository(pool *pgxpool.Pool, opts ...RepositoryOptions) *AuditLogRepository {
	observer := observability.ProviderObserver(observability.NoopProviderObserver{})
	if len(opts) > 0 && opts[0].Observer != nil {
		observer = opts[0].Observer
	}
	return &AuditLogRepository{pool: pool, queries: sqlc.New(pool), observer: observer}
}

func (repository *AuditLogRepository) observe(operation string, startedAt time.Time, err error) {
	repository.observer.Observe("postgres", operation, time.Since(startedAt), err)
}

// Append inserts a new entry.
func (repository *AuditLogRepository) Append(ctx context.Context, entry domain.AuditEntry) (err error) {
	startedAt := time.Now()
	defer func() { repository.observe("append_audit", startedAt, err) }()

	return repository.queries.AppendAuditEntry(ctx, sqlc.AppendAuditEntryParams{
		RequestID:  entry.RequestID,
		ApiKeyID:   entry.APIKeyID,
		Actor:      entry.Actor,
		Method:     entry.Method,
		Path:       entry.Path,
		Status:     int32(entry.Status),
		DurationMs: entry.DurationMs,
		Remote:     entry.Remote,
		CreatedAt:  pgtype.Timestamptz{Time: entry.CreatedAt, Valid: !entry.CreatedAt.IsZero()},
	})
}

// List returns audit entries honoring the supplied filter, newest first.
func (repository *AuditLogRepository) List(ctx context.Context, filter domain.AuditFilter) (entries []domain.AuditEntry, err error) {
	startedAt := time.Now()
	defer func() { repository.observe("list_audit", startedAt, err) }()

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}

	rows, err := repository.queries.ListAuditEntries(ctx, sqlc.ListAuditEntriesParams{
		ApiKeyID:     optionalText(strings.TrimSpace(filter.APIKeyID)),
		FromAt:       optionalTimestamp(timeOrNil(filter.From)),
		ToAt:         optionalTimestamp(timeOrNil(filter.To)),
		ResultLimit:  int32(limit),
		ResultOffset: int32(filter.Offset),
	})
	if err != nil {
		return nil, fmt.Errorf("list audit entries: %w", err)
	}
	out := make([]domain.AuditEntry, 0, len(rows))
	for _, row := range rows {
		entry := domain.AuditEntry{
			ID:         row.ID,
			RequestID:  row.RequestID,
			APIKeyID:   row.ApiKeyID,
			Actor:      row.Actor,
			Method:     row.Method,
			Path:       row.Path,
			Status:     int(row.Status),
			DurationMs: row.DurationMs,
			Remote:     row.Remote,
		}
		if row.CreatedAt.Valid {
			entry.CreatedAt = row.CreatedAt.Time.UTC()
		}
		out = append(out, entry)
	}
	return out, nil
}
