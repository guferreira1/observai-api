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

// APIKeyRepository persists API keys in PostgreSQL.
type APIKeyRepository struct {
	pool     *pgxpool.Pool
	queries  *sqlc.Queries
	observer observability.ProviderObserver
}

// NewAPIKeyRepository creates a PostgreSQL API key repository sharing the
// pool with other repositories.
func NewAPIKeyRepository(pool *pgxpool.Pool, opts ...RepositoryOptions) *APIKeyRepository {
	observer := observability.ProviderObserver(observability.NoopProviderObserver{})
	if len(opts) > 0 && opts[0].Observer != nil {
		observer = opts[0].Observer
	}
	return &APIKeyRepository{
		pool:     pool,
		queries:  sqlc.New(pool),
		observer: observer,
	}
}

func (repository *APIKeyRepository) observe(operation string, startedAt time.Time, err error) {
	repository.observer.Observe("postgres", operation, time.Since(startedAt), err)
}

// Create stores a new API key.
func (repository *APIKeyRepository) Create(ctx context.Context, key domain.APIKey) (err error) {
	startedAt := time.Now()
	defer func() { repository.observe("create_api_key", startedAt, err) }()

	return repository.queries.CreateAPIKey(ctx, sqlc.CreateAPIKeyParams{
		ID:        key.ID,
		Name:      key.Name,
		KeyHash:   key.Hash,
		Scope:     string(key.Scope),
		CreatedAt: pgtype.Timestamptz{Time: key.CreatedAt, Valid: !key.CreatedAt.IsZero()},
	})
}

// FindByHash returns the API key matching the supplied hash. Revoked keys
// are ignored (treated as not found).
func (repository *APIKeyRepository) FindByHash(ctx context.Context, hash string) (key domain.APIKey, err error) {
	startedAt := time.Now()
	defer func() { repository.observe("find_api_key_by_hash", startedAt, err) }()

	row, err := repository.queries.FindAPIKeyByHash(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.APIKey{}, fmt.Errorf("%w", domain.ErrAPIKeyNotFound)
	}
	if err != nil {
		return domain.APIKey{}, fmt.Errorf("find api key: %w", err)
	}
	return toDomainAPIKey(row.ID, row.Name, row.KeyHash, row.Scope, row.CreatedAt, row.LastUsedAt, row.RevokedAt), nil
}

// List returns persisted API keys ordered by creation time, newest first.
func (repository *APIKeyRepository) List(ctx context.Context, limit int, offset int) (keys []domain.APIKey, err error) {
	startedAt := time.Now()
	defer func() { repository.observe("list_api_keys", startedAt, err) }()

	rows, err := repository.queries.ListAPIKeys(ctx, sqlc.ListAPIKeysParams{
		ResultLimit:  int32(limit),
		ResultOffset: int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	out := make([]domain.APIKey, 0, len(rows))
	for _, row := range rows {
		out = append(out, toDomainAPIKey(row.ID, row.Name, row.KeyHash, row.Scope, row.CreatedAt, row.LastUsedAt, row.RevokedAt))
	}
	return out, nil
}

// Revoke marks the supplied key as revoked.
func (repository *APIKeyRepository) Revoke(ctx context.Context, id string) (err error) {
	startedAt := time.Now()
	defer func() { repository.observe("revoke_api_key", startedAt, err) }()

	return repository.queries.RevokeAPIKey(ctx, sqlc.RevokeAPIKeyParams{
		ID:        id,
		RevokedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	})
}

// TouchLastUsed updates the last_used_at timestamp for an active key.
func (repository *APIKeyRepository) TouchLastUsed(ctx context.Context, id string) (err error) {
	startedAt := time.Now()
	defer func() { repository.observe("touch_api_key_last_used", startedAt, err) }()

	return repository.queries.TouchAPIKeyLastUsed(ctx, sqlc.TouchAPIKeyLastUsedParams{
		ID:         id,
		LastUsedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	})
}

func toDomainAPIKey(id, name, hash, scope string, createdAt, lastUsedAt, revokedAt pgtype.Timestamptz) domain.APIKey {
	key := domain.APIKey{
		ID:    id,
		Name:  name,
		Hash:  hash,
		Scope: domain.APIKeyScope(scope),
	}
	if createdAt.Valid {
		key.CreatedAt = createdAt.Time.UTC()
	}
	if lastUsedAt.Valid {
		stamp := lastUsedAt.Time.UTC()
		key.LastUsedAt = &stamp
	}
	if revokedAt.Valid {
		stamp := revokedAt.Time.UTC()
		key.RevokedAt = &stamp
	}
	return key
}
