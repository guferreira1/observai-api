package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/guferreira1/observai-api/internal/adapters/outbound/postgres/sqlc"
	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AnalysisRepository stores analysis results in PostgreSQL.
type AnalysisRepository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

// NewAnalysisRepository creates a PostgreSQL analysis repository.
func NewAnalysisRepository(ctx context.Context, dsn string) (*AnalysisRepository, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return &AnalysisRepository{
		pool:    pool,
		queries: sqlc.New(pool),
	}, nil
}

// Close releases PostgreSQL connections held by the repository.
func (repository *AnalysisRepository) Close() {
	repository.pool.Close()
}

// Save stores an analysis result by identifier.
func (repository *AnalysisRepository) Save(ctx context.Context, result domain.AnalysisResult) error {
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
func (repository *AnalysisRepository) Find(ctx context.Context, id string) (domain.AnalysisResult, error) {
	row, err := repository.queries.FindAnalysis(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.AnalysisResult{}, fmt.Errorf("%w: %s", domain.ErrAnalysisNotFound, id)
	}
	if err != nil {
		return domain.AnalysisResult{}, fmt.Errorf("select analysis: %w", err)
	}

	return toDomainAnalysisResult(row)
}

// SaveExchange stores a user question and assistant answer in a single transaction.
func (repository *AnalysisRepository) SaveExchange(ctx context.Context, question domain.ChatMessage, answer domain.ChatMessage) error {
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
func (repository *AnalysisRepository) List(ctx context.Context, analysisID string) ([]domain.ChatMessage, error) {
	rows, err := repository.queries.ListChatMessagesByAnalysis(ctx, analysisID)
	if err != nil {
		return nil, fmt.Errorf("select chat history: %w", err)
	}

	messages := make([]domain.ChatMessage, 0, len(rows))
	for _, row := range rows {
		message, err := toDomainChatMessage(row)
		if err != nil {
			return nil, err
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
