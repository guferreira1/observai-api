package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/guferreira1/observai-api/internal/adapters/outbound/postgres/sqlc"
	"github.com/guferreira1/observai-api/internal/platform/observability"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AnalysisRetentionRepository implements destructive retention operations.
type AnalysisRetentionRepository struct {
	pool     *pgxpool.Pool
	queries  *sqlc.Queries
	observer observability.ProviderObserver
}

// NewAnalysisRetentionRepository builds a retention repository sharing the pool.
func NewAnalysisRetentionRepository(pool *pgxpool.Pool, opts ...RepositoryOptions) *AnalysisRetentionRepository {
	observer := observability.ProviderObserver(observability.NoopProviderObserver{})
	if len(opts) > 0 && opts[0].Observer != nil {
		observer = opts[0].Observer
	}
	return &AnalysisRetentionRepository{pool: pool, queries: sqlc.New(pool), observer: observer}
}

func (repository *AnalysisRetentionRepository) observe(operation string, startedAt time.Time, err error) {
	repository.observer.Observe("postgres", operation, time.Since(startedAt), err)
}

// DeleteByID removes the analysis and cascades to chat history / feedback / jobs.
func (repository *AnalysisRetentionRepository) DeleteByID(ctx context.Context, id string) (rowsAffected int, err error) {
	startedAt := time.Now()
	defer func() { repository.observe("delete_analysis_by_id", startedAt, err) }()

	affected, err := repository.queries.DeleteAnalysisByID(ctx, id)
	if err != nil {
		return 0, fmt.Errorf("delete analysis: %w", err)
	}
	return int(affected), nil
}

// DeleteOlderThan removes every analysis whose created_at is strictly before cutoff.
func (repository *AnalysisRetentionRepository) DeleteOlderThan(ctx context.Context, cutoff time.Time) (rowsAffected int, err error) {
	startedAt := time.Now()
	defer func() { repository.observe("delete_analyses_older_than", startedAt, err) }()

	affected, err := repository.queries.DeleteAnalysesOlderThan(ctx, pgtype.Timestamptz{Time: cutoff.UTC(), Valid: true})
	if err != nil {
		return 0, fmt.Errorf("delete analyses older than: %w", err)
	}
	return int(affected), nil
}

// DeleteKeepingNewest preserves the supplied number of newest analyses and
// removes everything else (FK cascade cleans chat history, feedback and
// jobs). A non-positive keep leaves the table untouched.
func (repository *AnalysisRetentionRepository) DeleteKeepingNewest(ctx context.Context, keep int) (rowsAffected int, err error) {
	startedAt := time.Now()
	defer func() { repository.observe("delete_analyses_keeping_newest", startedAt, err) }()

	if keep <= 0 {
		return 0, nil
	}
	affected, err := repository.queries.DeleteAnalysesKeepingNewest(ctx, int32(keep))
	if err != nil {
		return 0, fmt.Errorf("delete analyses keeping newest: %w", err)
	}
	return int(affected), nil
}
