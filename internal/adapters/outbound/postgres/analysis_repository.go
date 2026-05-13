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
	"github.com/jackc/pgx/v5/pgxpool"
)

// AnalysisRepository stores analysis results in PostgreSQL.
type AnalysisRepository struct {
	pool     *pgxpool.Pool
	queries  *sqlc.Queries
	observer observability.ProviderObserver
}

// RepositoryOptions configures optional collaborators for the repository.
type RepositoryOptions struct {
	Observer observability.ProviderObserver
}

// NewAnalysisRepository creates a PostgreSQL analysis repository.
func NewAnalysisRepository(ctx context.Context, dsn string, opts ...RepositoryOptions) (*AnalysisRepository, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	observer := observability.ProviderObserver(observability.NoopProviderObserver{})
	if len(opts) > 0 && opts[0].Observer != nil {
		observer = opts[0].Observer
	}

	return &AnalysisRepository{
		pool:     pool,
		queries:  sqlc.New(pool),
		observer: observer,
	}, nil
}

// Ping verifies connectivity to PostgreSQL with the supplied context.
func (repository *AnalysisRepository) Ping(ctx context.Context) error {
	return repository.pool.Ping(ctx)
}

// Pool exposes the underlying connection pool so sibling repositories can share it.
func (repository *AnalysisRepository) Pool() *pgxpool.Pool {
	return repository.pool
}

func (repository *AnalysisRepository) observe(operation string, startedAt time.Time, err error) {
	repository.observer.Observe("postgres", operation, time.Since(startedAt), err)
}

// Close releases PostgreSQL connections held by the repository.
func (repository *AnalysisRepository) Close() {
	repository.pool.Close()
}

// Save stores an analysis result by identifier.
func (repository *AnalysisRepository) Save(ctx context.Context, result domain.AnalysisResult) (err error) {
	startedAt := time.Now()
	defer func() { repository.observe("save_analysis", startedAt, err) }()

	params, err := toSaveAnalysisParams(result)
	if err != nil {
		return err
	}

	if err := repository.queries.SaveAnalysis(ctx, params); err != nil {
		return fmt.Errorf("insert analysis: %w", err)
	}

	return nil
}

// Find returns an analysis result by identifier.
func (repository *AnalysisRepository) Find(ctx context.Context, id string) (result domain.AnalysisResult, err error) {
	startedAt := time.Now()
	defer func() { repository.observe("find_analysis", startedAt, err) }()

	row, err := repository.queries.FindAnalysis(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.AnalysisResult{}, fmt.Errorf("%w: %s", domain.ErrAnalysisNotFound, id)
	}
	if err != nil {
		return domain.AnalysisResult{}, fmt.Errorf("select analysis: %w", err)
	}

	return toDomainAnalysisResult(row)
}

// ListAnalyses returns analyses ordered by creation time descending.
func (repository *AnalysisRepository) ListAnalyses(ctx context.Context, filter domain.AnalysisListFilter) (list domain.AnalysisList, err error) {
	startedAt := time.Now()
	defer func() { repository.observe("list_analyses", startedAt, err) }()

	queryFilter := toAnalysisFilterParams(filter)
	total, err := repository.queries.CountAnalyses(ctx, sqlc.CountAnalysesParams{
		Severity: queryFilter.severity,
		Service:  queryFilter.service,
	})
	if err != nil {
		return domain.AnalysisList{}, fmt.Errorf("count analyses: %w", err)
	}

	rows, err := repository.queries.ListAnalyses(ctx, sqlc.ListAnalysesParams{
		Severity:     queryFilter.severity,
		Service:      queryFilter.service,
		ResultLimit:  int32(filter.Limit),
		ResultOffset: int32(filter.Offset),
	})
	if err != nil {
		return domain.AnalysisList{}, fmt.Errorf("select analyses: %w", err)
	}

	items := make([]domain.AnalysisResult, 0, len(rows))
	for _, row := range rows {
		mapped, mapErr := toDomainAnalysisResultFromList(row)
		if mapErr != nil {
			return domain.AnalysisList{}, mapErr
		}
		items = append(items, mapped)
	}

	return domain.AnalysisList{
		Items:  items,
		Limit:  filter.Limit,
		Offset: filter.Offset,
		Total:  int(total),
	}, nil
}

// SaveExchange stores a user question and assistant answer in a single transaction.
func (repository *AnalysisRepository) SaveExchange(ctx context.Context, question domain.ChatMessage, answer domain.ChatMessage) (err error) {
	startedAt := time.Now()
	defer func() { repository.observe("save_chat_exchange", startedAt, err) }()

	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin chat history transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	queries := repository.queries.WithTx(tx)
	if _, err := createChatMessage(ctx, queries, question); err != nil {
		return fmt.Errorf("insert user chat message: %w", err)
	}

	if _, err := createChatMessage(ctx, queries, answer); err != nil {
		return fmt.Errorf("insert assistant chat message: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit chat history transaction: %w", err)
	}

	return nil
}

// List returns persisted chat messages for an analysis.
func (repository *AnalysisRepository) List(ctx context.Context, analysisID string) (messages []domain.ChatMessage, err error) {
	startedAt := time.Now()
	defer func() { repository.observe("list_chat_messages", startedAt, err) }()

	rows, err := repository.queries.ListChatMessagesByAnalysis(ctx, analysisID)
	if err != nil {
		return nil, fmt.Errorf("select chat history: %w", err)
	}

	messages = make([]domain.ChatMessage, 0, len(rows))
	for _, row := range rows {
		message, mapErr := toDomainChatMessage(row)
		if mapErr != nil {
			return nil, mapErr
		}

		messages = append(messages, message)
	}

	return messages, nil
}

func createChatMessage(ctx context.Context, queries *sqlc.Queries, message domain.ChatMessage) (domain.ChatMessage, error) {
	params, err := toCreateChatMessageParams(message)
	if err != nil {
		return domain.ChatMessage{}, err
	}

	row, err := queries.CreateChatMessage(ctx, params)
	if err != nil {
		return domain.ChatMessage{}, err
	}

	return toDomainChatMessage(row)
}
