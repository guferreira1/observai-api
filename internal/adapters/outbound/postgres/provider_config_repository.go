package postgres

import (
	"context"
	"encoding/json"
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

// ProviderConfigRepository persists observability provider configurations
// in PostgreSQL.
type ProviderConfigRepository struct {
	pool     *pgxpool.Pool
	queries  *sqlc.Queries
	observer observability.ProviderObserver
}

// NewProviderConfigRepository creates a PostgreSQL provider-config
// repository sharing the pool with other repositories.
func NewProviderConfigRepository(pool *pgxpool.Pool, opts ...RepositoryOptions) *ProviderConfigRepository {
	observer := observability.ProviderObserver(observability.NoopProviderObserver{})
	if len(opts) > 0 && opts[0].Observer != nil {
		observer = opts[0].Observer
	}
	return &ProviderConfigRepository{pool: pool, queries: sqlc.New(pool), observer: observer}
}

func (repository *ProviderConfigRepository) observe(operation string, startedAt time.Time, err error) {
	repository.observer.Observe("postgres", operation, time.Since(startedAt), err)
}

// Create persists a new provider configuration.
func (repository *ProviderConfigRepository) Create(ctx context.Context, config domain.ProviderConfig) (err error) {
	startedAt := time.Now()
	defer func() { repository.observe("create_provider_config", startedAt, err) }()

	options, err := encodeJSONMap(config.Options)
	if err != nil {
		return fmt.Errorf("encode provider options: %w", err)
	}
	params := sqlc.CreateProviderConfigParams{
		ID:        config.ID,
		Type:      config.Type,
		Name:      config.Name,
		Url:       config.URL,
		TimeoutMs: int32(config.Timeout / time.Millisecond),
		Signals:   config.Signals,
		Options:   options,
		IsActive:  config.IsActive,
		CreatedAt: pgtype.Timestamptz{Time: config.CreatedAt, Valid: !config.CreatedAt.IsZero()},
		UpdatedAt: pgtype.Timestamptz{Time: config.UpdatedAt, Valid: !config.UpdatedAt.IsZero()},
	}
	if config.CredentialsCiphertext != "" {
		params.CredentialsCiphertext = pgtype.Text{String: config.CredentialsCiphertext, Valid: true}
	}
	err = repository.queries.CreateProviderConfig(ctx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrProviderConfigConflict
		}
		return fmt.Errorf("create provider config: %w", err)
	}
	return nil
}

// Find returns the provider configuration matching the supplied identifier.
func (repository *ProviderConfigRepository) Find(ctx context.Context, id string) (config domain.ProviderConfig, err error) {
	startedAt := time.Now()
	defer func() { repository.observe("find_provider_config", startedAt, err) }()

	row, err := repository.queries.FindProviderConfig(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ProviderConfig{}, domain.ErrProviderConfigNotFound
	}
	if err != nil {
		return domain.ProviderConfig{}, fmt.Errorf("find provider config: %w", err)
	}
	return rowToProviderConfig(row.ID, row.Type, row.Name, row.Url, row.TimeoutMs, row.Signals, row.Options, row.CredentialsCiphertext, row.IsActive, row.CreatedAt, row.UpdatedAt)
}

// List returns persisted configurations ordered by creation time, newest first.
func (repository *ProviderConfigRepository) List(ctx context.Context, limit int, offset int) (configs []domain.ProviderConfig, err error) {
	startedAt := time.Now()
	defer func() { repository.observe("list_provider_configs", startedAt, err) }()

	rows, err := repository.queries.ListProviderConfigs(ctx, sqlc.ListProviderConfigsParams{
		ResultLimit:  int32(limit),
		ResultOffset: int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("list provider configs: %w", err)
	}
	out := make([]domain.ProviderConfig, 0, len(rows))
	for _, row := range rows {
		config, err := rowToProviderConfig(row.ID, row.Type, row.Name, row.Url, row.TimeoutMs, row.Signals, row.Options, row.CredentialsCiphertext, row.IsActive, row.CreatedAt, row.UpdatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, config)
	}
	return out, nil
}

// ListActive returns provider configurations with is_active = true.
func (repository *ProviderConfigRepository) ListActive(ctx context.Context) (configs []domain.ProviderConfig, err error) {
	startedAt := time.Now()
	defer func() { repository.observe("list_active_provider_configs", startedAt, err) }()

	rows, err := repository.queries.ListActiveProviderConfigs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active provider configs: %w", err)
	}
	out := make([]domain.ProviderConfig, 0, len(rows))
	for _, row := range rows {
		config, err := rowToProviderConfig(row.ID, row.Type, row.Name, row.Url, row.TimeoutMs, row.Signals, row.Options, row.CredentialsCiphertext, row.IsActive, row.CreatedAt, row.UpdatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, config)
	}
	return out, nil
}

// Count returns the total number of persisted provider configurations.
func (repository *ProviderConfigRepository) Count(ctx context.Context) (count int64, err error) {
	startedAt := time.Now()
	defer func() { repository.observe("count_provider_configs", startedAt, err) }()
	return repository.queries.CountProviderConfigs(ctx)
}

// Update replaces a stored provider configuration.
func (repository *ProviderConfigRepository) Update(ctx context.Context, config domain.ProviderConfig) (err error) {
	startedAt := time.Now()
	defer func() { repository.observe("update_provider_config", startedAt, err) }()

	options, err := encodeJSONMap(config.Options)
	if err != nil {
		return fmt.Errorf("encode provider options: %w", err)
	}
	params := sqlc.UpdateProviderConfigParams{
		ID:        config.ID,
		Name:      config.Name,
		Url:       config.URL,
		TimeoutMs: int32(config.Timeout / time.Millisecond),
		Signals:   config.Signals,
		Options:   options,
		UpdatedAt: pgtype.Timestamptz{Time: config.UpdatedAt, Valid: !config.UpdatedAt.IsZero()},
	}
	if config.CredentialsCiphertext != "" {
		params.CredentialsCiphertext = pgtype.Text{String: config.CredentialsCiphertext, Valid: true}
	}
	err = repository.queries.UpdateProviderConfig(ctx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrProviderConfigConflict
		}
		return fmt.Errorf("update provider config: %w", err)
	}
	return nil
}

// SetActive toggles the is_active flag.
func (repository *ProviderConfigRepository) SetActive(ctx context.Context, id string, active bool, when time.Time) (err error) {
	startedAt := time.Now()
	defer func() { repository.observe("set_provider_config_active", startedAt, err) }()
	return repository.queries.SetProviderConfigActive(ctx, sqlc.SetProviderConfigActiveParams{
		ID:        id,
		IsActive:  active,
		UpdatedAt: pgtype.Timestamptz{Time: when, Valid: true},
	})
}

// Delete removes a provider configuration permanently.
func (repository *ProviderConfigRepository) Delete(ctx context.Context, id string) (err error) {
	startedAt := time.Now()
	defer func() { repository.observe("delete_provider_config", startedAt, err) }()
	return repository.queries.DeleteProviderConfig(ctx, id)
}

func rowToProviderConfig(id, providerType, name, url string, timeoutMs int32, signals []string, options []byte, credentials pgtype.Text, isActive bool, createdAt, updatedAt pgtype.Timestamptz) (domain.ProviderConfig, error) {
	config := domain.ProviderConfig{
		ID:       id,
		Type:     providerType,
		Name:     name,
		URL:      url,
		Timeout:  time.Duration(timeoutMs) * time.Millisecond,
		Signals:  signals,
		IsActive: isActive,
	}
	if credentials.Valid {
		config.CredentialsCiphertext = credentials.String
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
			return domain.ProviderConfig{}, fmt.Errorf("decode provider options: %w", err)
		}
		config.Options = opts
	}
	return config, nil
}

func encodeJSONMap(values map[string]string) ([]byte, error) {
	if len(values) == 0 {
		return []byte("{}"), nil
	}
	return json.Marshal(values)
}

func decodeJSONMap(payload []byte) (map[string]string, error) {
	if len(payload) == 0 {
		return nil, nil
	}
	out := make(map[string]string)
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil, err
	}
	return out, nil
}
