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

	scopes := make([]string, 0, len(key.Scopes))
	for _, scope := range key.Scopes {
		scopes = append(scopes, string(scope))
	}

	params := sqlc.CreateAPIKeyParams{
		ID:        key.ID,
		Name:      key.Name,
		KeyHash:   key.Hash,
		Scope:     legacyScopeFromScopes(key.Scopes),
		Scopes:    scopes,
		CreatedAt: pgtype.Timestamptz{Time: key.CreatedAt, Valid: !key.CreatedAt.IsZero()},
	}
	if key.Description != "" {
		params.Description = pgtype.Text{String: key.Description, Valid: true}
	}
	if key.ExpiresAt != nil {
		params.ExpiresAt = pgtype.Timestamptz{Time: *key.ExpiresAt, Valid: true}
	}
	return repository.queries.CreateAPIKey(ctx, params)
}

// FindByHash returns the API key matching the supplied hash, including
// revoked or expired entries so the use case can classify them.
func (repository *APIKeyRepository) FindByHash(ctx context.Context, hash string) (key domain.APIKey, err error) {
	startedAt := time.Now()
	defer func() { repository.observe("find_api_key_by_hash", startedAt, err) }()

	row, err := repository.queries.FindAPIKeyByHash(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.APIKey{}, domain.ErrAPIKeyNotFound
	}
	if err != nil {
		return domain.APIKey{}, fmt.Errorf("find api key: %w", err)
	}
	return rowToDomainAPIKey(row.ID, row.Name, row.KeyHash, row.Scope, row.Scopes, row.Description, row.ExpiresAt, row.CreatedAt, row.LastUsedAt, row.RevokedAt), nil
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
		out = append(out, rowToDomainAPIKey(row.ID, row.Name, row.KeyHash, row.Scope, row.Scopes, row.Description, row.ExpiresAt, row.CreatedAt, row.LastUsedAt, row.RevokedAt))
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

func rowToDomainAPIKey(id, name, hash, _ string, scopes []string, description pgtype.Text, expiresAt, createdAt, lastUsedAt, revokedAt pgtype.Timestamptz) domain.APIKey {
	key := domain.APIKey{
		ID:   id,
		Name: name,
		Hash: hash,
	}
	if description.Valid {
		key.Description = description.String
	}
	for _, scope := range scopes {
		key.Scopes = append(key.Scopes, domain.APIKeyScope(scope))
	}
	if createdAt.Valid {
		key.CreatedAt = createdAt.Time.UTC()
	}
	if expiresAt.Valid {
		stamp := expiresAt.Time.UTC()
		key.ExpiresAt = &stamp
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

// legacyScopeFromScopes derives a value for the legacy `scope` column from
// the fine-grained scope set. The column is no longer authoritative but is
// kept populated so downgrades and historical queries continue to work.
func legacyScopeFromScopes(scopes []domain.APIKeyScope) string {
	for _, scope := range scopes {
		if scope == domain.APIKeyScopeAdminWrite || scope == domain.APIKeyScopeAdminRead {
			return "admin"
		}
	}
	return "default"
}
