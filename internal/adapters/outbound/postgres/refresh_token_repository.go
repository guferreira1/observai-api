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

// RefreshTokenRepository persists refresh tokens in PostgreSQL.
type RefreshTokenRepository struct {
	pool     *pgxpool.Pool
	queries  *sqlc.Queries
	observer observability.ProviderObserver
}

// NewRefreshTokenRepository creates a PostgreSQL refresh-token repository
// sharing the pool with other repositories.
func NewRefreshTokenRepository(pool *pgxpool.Pool, opts ...RepositoryOptions) *RefreshTokenRepository {
	observer := observability.ProviderObserver(observability.NoopProviderObserver{})
	if len(opts) > 0 && opts[0].Observer != nil {
		observer = opts[0].Observer
	}
	return &RefreshTokenRepository{pool: pool, queries: sqlc.New(pool), observer: observer}
}

func (repository *RefreshTokenRepository) observe(operation string, startedAt time.Time, err error) {
	repository.observer.Observe("postgres", operation, time.Since(startedAt), err)
}

// Create persists a new refresh token.
func (repository *RefreshTokenRepository) Create(ctx context.Context, token domain.RefreshToken) (err error) {
	startedAt := time.Now()
	defer func() { repository.observe("create_refresh_token", startedAt, err) }()
	return repository.queries.CreateRefreshToken(ctx, sqlc.CreateRefreshTokenParams{
		ID:        token.ID,
		UserID:    token.UserID,
		TokenHash: token.TokenHash,
		FamilyID:  token.FamilyID,
		ExpiresAt: pgtype.Timestamptz{Time: token.ExpiresAt, Valid: true},
		CreatedAt: pgtype.Timestamptz{Time: token.CreatedAt, Valid: !token.CreatedAt.IsZero()},
	})
}

// FindByHash returns the refresh token matching the supplied hash, including
// revoked or expired entries so callers can detect reuse attacks.
func (repository *RefreshTokenRepository) FindByHash(ctx context.Context, hash string) (token domain.RefreshToken, err error) {
	startedAt := time.Now()
	defer func() { repository.observe("find_refresh_token_by_hash", startedAt, err) }()

	row, err := repository.queries.FindRefreshTokenByHash(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.RefreshToken{}, domain.ErrInvalidRefreshToken
	}
	if err != nil {
		return domain.RefreshToken{}, fmt.Errorf("find refresh token: %w", err)
	}
	out := domain.RefreshToken{
		ID:        row.ID,
		UserID:    row.UserID,
		TokenHash: row.TokenHash,
		FamilyID:  row.FamilyID,
	}
	if row.ExpiresAt.Valid {
		out.ExpiresAt = row.ExpiresAt.Time.UTC()
	}
	if row.CreatedAt.Valid {
		out.CreatedAt = row.CreatedAt.Time.UTC()
	}
	if row.RevokedAt.Valid {
		stamp := row.RevokedAt.Time.UTC()
		out.RevokedAt = &stamp
	}
	if row.ReplacedBy.Valid {
		replaced := row.ReplacedBy.String
		out.ReplacedBy = &replaced
	}
	return out, nil
}

// Revoke marks the supplied token as revoked. When replacedBy is set the
// new token id is stored alongside the revocation for audit and rotation
// traceability.
func (repository *RefreshTokenRepository) Revoke(ctx context.Context, id string, replacedBy *string, when time.Time) (err error) {
	startedAt := time.Now()
	defer func() { repository.observe("revoke_refresh_token", startedAt, err) }()
	params := sqlc.RevokeRefreshTokenParams{
		ID:        id,
		RevokedAt: pgtype.Timestamptz{Time: when, Valid: true},
	}
	if replacedBy != nil {
		params.ReplacedBy = pgtype.Text{String: *replacedBy, Valid: true}
	}
	return repository.queries.RevokeRefreshToken(ctx, params)
}

// RevokeFamily marks every token in a family as revoked, used when reuse
// of a revoked token is detected.
func (repository *RefreshTokenRepository) RevokeFamily(ctx context.Context, familyID string, when time.Time) (err error) {
	startedAt := time.Now()
	defer func() { repository.observe("revoke_refresh_token_family", startedAt, err) }()
	return repository.queries.RevokeRefreshTokenFamily(ctx, sqlc.RevokeRefreshTokenFamilyParams{
		FamilyID:  familyID,
		RevokedAt: pgtype.Timestamptz{Time: when, Valid: true},
	})
}

// RevokeAllForUser revokes every active token for the supplied user. Used
// when an admin disables an account or the user logs out everywhere.
func (repository *RefreshTokenRepository) RevokeAllForUser(ctx context.Context, userID string, when time.Time) (err error) {
	startedAt := time.Now()
	defer func() { repository.observe("revoke_refresh_tokens_for_user", startedAt, err) }()
	return repository.queries.RevokeRefreshTokensForUser(ctx, sqlc.RevokeRefreshTokensForUserParams{
		UserID:    userID,
		RevokedAt: pgtype.Timestamptz{Time: when, Valid: true},
	})
}
