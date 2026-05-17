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
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LLMConfigRepository persists LLM provider configurations in PostgreSQL.
type LLMConfigRepository struct {
	pool     *pgxpool.Pool
	queries  *sqlc.Queries
	observer observability.ProviderObserver
}

// NewLLMConfigRepository creates a PostgreSQL LLM-config repository
// sharing the pool with other repositories.
func NewLLMConfigRepository(pool *pgxpool.Pool, opts ...RepositoryOptions) *LLMConfigRepository {
	observer := observability.ProviderObserver(observability.NoopProviderObserver{})
	if len(opts) > 0 && opts[0].Observer != nil {
		observer = opts[0].Observer
	}
	return &LLMConfigRepository{pool: pool, queries: sqlc.New(pool), observer: observer}
}

func (repository *LLMConfigRepository) observe(operation string, startedAt time.Time, err error) {
	repository.observer.Observe("postgres", operation, time.Since(startedAt), err)
}

// Create persists a new LLM configuration.
func (repository *LLMConfigRepository) Create(ctx context.Context, config domain.LLMConfig) (err error) {
	startedAt := time.Now()
	defer func() { repository.observe("create_llm_config", startedAt, err) }()

	options, err := encodeJSONMap(config.Options)
	if err != nil {
		return fmt.Errorf("encode llm options: %w", err)
	}
	params := sqlc.CreateLLMConfigParams{
		ID:        config.ID,
		Type:      config.Type,
		Name:      config.Name,
		BaseUrl:   config.BaseURL,
		Model:     config.Model,
		TimeoutMs: int32(config.Timeout / time.Millisecond),
		Options:   options,
		IsActive:  config.IsActive,
		CreatedAt: pgtype.Timestamptz{Time: config.CreatedAt, Valid: !config.CreatedAt.IsZero()},
		UpdatedAt: pgtype.Timestamptz{Time: config.UpdatedAt, Valid: !config.UpdatedAt.IsZero()},
	}
	if config.APIKeyCipher != "" {
		params.ApiKeyCiphertext = pgtype.Text{String: config.APIKeyCipher, Valid: true}
	}
	err = repository.queries.CreateLLMConfig(ctx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrProviderConfigConflict
		}
		return fmt.Errorf("create llm config: %w", err)
	}
	return nil
}

// Find returns the LLM configuration matching the supplied identifier.
func (repository *LLMConfigRepository) Find(ctx context.Context, id string) (config domain.LLMConfig, err error) {
	startedAt := time.Now()
	defer func() { repository.observe("find_llm_config", startedAt, err) }()

	row, err := repository.queries.FindLLMConfig(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.LLMConfig{}, domain.ErrLLMConfigNotFound
	}
	if err != nil {
		return domain.LLMConfig{}, fmt.Errorf("find llm config: %w", err)
	}
	return rowToLLMConfig(row.ID, row.Type, row.Name, row.BaseUrl, row.Model, row.TimeoutMs, row.Options, row.ApiKeyCiphertext, row.IsActive, row.CreatedAt, row.UpdatedAt)
}

// List returns persisted LLM configurations ordered by creation time, newest first.
func (repository *LLMConfigRepository) List(ctx context.Context, limit int, offset int) (configs []domain.LLMConfig, err error) {
	startedAt := time.Now()
	defer func() { repository.observe("list_llm_configs", startedAt, err) }()

	rows, err := repository.queries.ListLLMConfigs(ctx, sqlc.ListLLMConfigsParams{
		ResultLimit:  int32(limit),
		ResultOffset: int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("list llm configs: %w", err)
	}
	out := make([]domain.LLMConfig, 0, len(rows))
	for _, row := range rows {
		config, err := rowToLLMConfig(row.ID, row.Type, row.Name, row.BaseUrl, row.Model, row.TimeoutMs, row.Options, row.ApiKeyCiphertext, row.IsActive, row.CreatedAt, row.UpdatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, config)
	}
	return out, nil
}

// FindActive returns the active LLM configuration, when one is set.
func (repository *LLMConfigRepository) FindActive(ctx context.Context) (config domain.LLMConfig, err error) {
	startedAt := time.Now()
	defer func() { repository.observe("find_active_llm_config", startedAt, err) }()

	row, err := repository.queries.FindActiveLLMConfig(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.LLMConfig{}, domain.ErrLLMConfigNotFound
	}
	if err != nil {
		return domain.LLMConfig{}, fmt.Errorf("find active llm config: %w", err)
	}
	return rowToLLMConfig(row.ID, row.Type, row.Name, row.BaseUrl, row.Model, row.TimeoutMs, row.Options, row.ApiKeyCiphertext, row.IsActive, row.CreatedAt, row.UpdatedAt)
}

// Count returns the total number of persisted LLM configurations.
func (repository *LLMConfigRepository) Count(ctx context.Context) (count int64, err error) {
	startedAt := time.Now()
	defer func() { repository.observe("count_llm_configs", startedAt, err) }()
	return repository.queries.CountLLMConfigs(ctx)
}

// Update replaces a stored LLM configuration.
func (repository *LLMConfigRepository) Update(ctx context.Context, config domain.LLMConfig) (err error) {
	startedAt := time.Now()
	defer func() { repository.observe("update_llm_config", startedAt, err) }()

	options, err := encodeJSONMap(config.Options)
	if err != nil {
		return fmt.Errorf("encode llm options: %w", err)
	}
	params := sqlc.UpdateLLMConfigParams{
		ID:        config.ID,
		Name:      config.Name,
		BaseUrl:   config.BaseURL,
		Model:     config.Model,
		TimeoutMs: int32(config.Timeout / time.Millisecond),
		Options:   options,
		UpdatedAt: pgtype.Timestamptz{Time: config.UpdatedAt, Valid: !config.UpdatedAt.IsZero()},
	}
	if config.APIKeyCipher != "" {
		params.ApiKeyCiphertext = pgtype.Text{String: config.APIKeyCipher, Valid: true}
	}
	err = repository.queries.UpdateLLMConfig(ctx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrProviderConfigConflict
		}
		return fmt.Errorf("update llm config: %w", err)
	}
	return nil
}

// Activate marks the supplied LLM configuration as active, deactivating
// every other entry inside the same transaction so the partial unique
// index never blocks the swap.
func (repository *LLMConfigRepository) Activate(ctx context.Context, id string, when time.Time) (err error) {
	startedAt := time.Now()
	defer func() { repository.observe("activate_llm_config", startedAt, err) }()

	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()
	queries := repository.queries.WithTx(tx)
	if err = queries.DeactivateAllLLMConfigs(ctx, pgtype.Timestamptz{Time: when, Valid: true}); err != nil {
		return fmt.Errorf("deactivate llm configs: %w", err)
	}
	if err = queries.ActivateLLMConfig(ctx, sqlc.ActivateLLMConfigParams{
		ID:        id,
		UpdatedAt: pgtype.Timestamptz{Time: when, Valid: true},
	}); err != nil {
		return fmt.Errorf("activate llm config: %w", err)
	}
	return tx.Commit(ctx)
}

// Delete removes the LLM configuration permanently.
func (repository *LLMConfigRepository) Delete(ctx context.Context, id string) (err error) {
	startedAt := time.Now()
	defer func() { repository.observe("delete_llm_config", startedAt, err) }()
	return repository.queries.DeleteLLMConfig(ctx, id)
}

func rowToLLMConfig(id, providerType, name, baseURL, model string, timeoutMs int32, options []byte, apiKey pgtype.Text, isActive bool, createdAt, updatedAt pgtype.Timestamptz) (domain.LLMConfig, error) {
	config := domain.LLMConfig{
		ID:       id,
		Type:     providerType,
		Name:     name,
		BaseURL:  baseURL,
		Model:    model,
		Timeout:  time.Duration(timeoutMs) * time.Millisecond,
		IsActive: isActive,
	}
	if apiKey.Valid {
		config.APIKeyCipher = apiKey.String
	}
	if createdAt.Valid {
		config.CreatedAt = createdAt.Time.UTC()
	}
	if updatedAt.Valid {
		config.UpdatedAt = updatedAt.Time.UTC()
	}
	if len(options) > 0 {
		opts, err := decodeJSONMap(options)
		if err != nil {
			return domain.LLMConfig{}, fmt.Errorf("decode llm options: %w", err)
		}
		config.Options = opts
	}
	return config, nil
}
