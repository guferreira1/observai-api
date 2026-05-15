package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/guferreira1/observai-api/internal/adapters/outbound/postgres/sqlc"
	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/platform/observability"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UserRepository persists application users in PostgreSQL.
type UserRepository struct {
	pool     *pgxpool.Pool
	queries  *sqlc.Queries
	observer observability.ProviderObserver
}

// NewUserRepository creates a PostgreSQL user repository sharing the pool
// with other repositories.
func NewUserRepository(pool *pgxpool.Pool, opts ...RepositoryOptions) *UserRepository {
	observer := observability.ProviderObserver(observability.NoopProviderObserver{})
	if len(opts) > 0 && opts[0].Observer != nil {
		observer = opts[0].Observer
	}
	return &UserRepository{pool: pool, queries: sqlc.New(pool), observer: observer}
}

func (repository *UserRepository) observe(operation string, startedAt time.Time, err error) {
	repository.observer.Observe("postgres", operation, time.Since(startedAt), err)
}

// Create persists a new user.
func (repository *UserRepository) Create(ctx context.Context, user domain.User) (err error) {
	startedAt := time.Now()
	defer func() { repository.observe("create_user", startedAt, err) }()

	err = repository.queries.CreateUser(ctx, sqlc.CreateUserParams{
		ID:                 user.ID,
		Email:              strings.ToLower(strings.TrimSpace(user.Email)),
		PasswordHash:       user.PasswordHash,
		Role:               string(user.Role),
		IsActive:           user.IsActive,
		MustChangePassword: user.MustChangePassword,
		Preferences:        marshalUserPreferences(user.Preferences),
		CreatedAt:          pgtype.Timestamptz{Time: user.CreatedAt, Valid: !user.CreatedAt.IsZero()},
		UpdatedAt:          pgtype.Timestamptz{Time: user.UpdatedAt, Valid: !user.UpdatedAt.IsZero()},
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrUserAlreadyExists
		}
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

// FindByID returns the user matching the supplied identifier.
func (repository *UserRepository) FindByID(ctx context.Context, id string) (user domain.User, err error) {
	startedAt := time.Now()
	defer func() { repository.observe("find_user_by_id", startedAt, err) }()

	row, err := repository.queries.FindUserByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("find user: %w", err)
	}
	return rowToDomainUser(row.ID, row.Email, row.PasswordHash, row.Role, row.IsActive, row.MustChangePassword, row.Preferences, row.CreatedAt, row.UpdatedAt, row.LastLoginAt), nil
}

// FindByEmail returns the user matching the supplied email (case-insensitive).
func (repository *UserRepository) FindByEmail(ctx context.Context, email string) (user domain.User, err error) {
	startedAt := time.Now()
	defer func() { repository.observe("find_user_by_email", startedAt, err) }()

	row, err := repository.queries.FindUserByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("find user by email: %w", err)
	}
	return rowToDomainUser(row.ID, row.Email, row.PasswordHash, row.Role, row.IsActive, row.MustChangePassword, row.Preferences, row.CreatedAt, row.UpdatedAt, row.LastLoginAt), nil
}

// List returns persisted users ordered by creation time, newest first.
func (repository *UserRepository) List(ctx context.Context, limit int, offset int) (users []domain.User, err error) {
	startedAt := time.Now()
	defer func() { repository.observe("list_users", startedAt, err) }()

	rows, err := repository.queries.ListUsers(ctx, sqlc.ListUsersParams{
		ResultLimit:  int32(limit),
		ResultOffset: int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	out := make([]domain.User, 0, len(rows))
	for _, row := range rows {
		out = append(out, rowToDomainUser(row.ID, row.Email, row.PasswordHash, row.Role, row.IsActive, row.MustChangePassword, row.Preferences, row.CreatedAt, row.UpdatedAt, row.LastLoginAt))
	}
	return out, nil
}

// Count returns the total number of persisted users.
func (repository *UserRepository) Count(ctx context.Context) (count int64, err error) {
	startedAt := time.Now()
	defer func() { repository.observe("count_users", startedAt, err) }()
	return repository.queries.CountUsers(ctx)
}

// UpdateProfile updates a user's email.
func (repository *UserRepository) UpdateProfile(ctx context.Context, id string, email string, updatedAt time.Time) (err error) {
	startedAt := time.Now()
	defer func() { repository.observe("update_user_profile", startedAt, err) }()

	err = repository.queries.UpdateUserProfile(ctx, sqlc.UpdateUserProfileParams{
		ID:        id,
		Email:     strings.ToLower(strings.TrimSpace(email)),
		UpdatedAt: pgtype.Timestamptz{Time: updatedAt, Valid: true},
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrUserAlreadyExists
		}
		return fmt.Errorf("update user profile: %w", err)
	}
	return nil
}

// UpdatePreferences stores the user's UI preference projection.
func (repository *UserRepository) UpdatePreferences(ctx context.Context, id string, preferences domain.UserPreferences, updatedAt time.Time) (err error) {
	startedAt := time.Now()
	defer func() { repository.observe("update_user_preferences", startedAt, err) }()

	return repository.queries.UpdateUserPreferences(ctx, sqlc.UpdateUserPreferencesParams{
		ID:          id,
		Preferences: marshalUserPreferences(preferences),
		UpdatedAt:   pgtype.Timestamptz{Time: updatedAt, Valid: true},
	})
}

// UpdatePassword replaces a user's password hash.
func (repository *UserRepository) UpdatePassword(ctx context.Context, id string, passwordHash string, updatedAt time.Time) (err error) {
	startedAt := time.Now()
	defer func() { repository.observe("update_user_password", startedAt, err) }()
	return repository.queries.UpdateUserPassword(ctx, sqlc.UpdateUserPasswordParams{
		ID:           id,
		PasswordHash: passwordHash,
		UpdatedAt:    pgtype.Timestamptz{Time: updatedAt, Valid: true},
	})
}

// UpdateRole replaces a user's role.
func (repository *UserRepository) UpdateRole(ctx context.Context, id string, role domain.Role, updatedAt time.Time) (err error) {
	startedAt := time.Now()
	defer func() { repository.observe("update_user_role", startedAt, err) }()
	return repository.queries.UpdateUserRole(ctx, sqlc.UpdateUserRoleParams{
		ID:        id,
		Role:      string(role),
		UpdatedAt: pgtype.Timestamptz{Time: updatedAt, Valid: true},
	})
}

// SetActive toggles a user's active flag.
func (repository *UserRepository) SetActive(ctx context.Context, id string, active bool, updatedAt time.Time) (err error) {
	startedAt := time.Now()
	defer func() { repository.observe("set_user_active", startedAt, err) }()
	return repository.queries.SetUserActive(ctx, sqlc.SetUserActiveParams{
		ID:        id,
		IsActive:  active,
		UpdatedAt: pgtype.Timestamptz{Time: updatedAt, Valid: true},
	})
}

// SetMustChangePassword toggles the forced password-change flag.
func (repository *UserRepository) SetMustChangePassword(ctx context.Context, id string, mustChangePassword bool, updatedAt time.Time) (err error) {
	startedAt := time.Now()
	defer func() { repository.observe("set_user_must_change_password", startedAt, err) }()
	return repository.queries.SetUserMustChangePassword(ctx, sqlc.SetUserMustChangePasswordParams{
		ID:                 id,
		MustChangePassword: mustChangePassword,
		UpdatedAt:          pgtype.Timestamptz{Time: updatedAt, Valid: true},
	})
}

// TouchLastLogin records the last successful login timestamp.
func (repository *UserRepository) TouchLastLogin(ctx context.Context, id string, when time.Time) (err error) {
	startedAt := time.Now()
	defer func() { repository.observe("touch_user_last_login", startedAt, err) }()
	return repository.queries.TouchUserLastLogin(ctx, sqlc.TouchUserLastLoginParams{
		ID:          id,
		LastLoginAt: pgtype.Timestamptz{Time: when, Valid: true},
		UpdatedAt:   pgtype.Timestamptz{Time: when, Valid: true},
	})
}

// Delete removes a user permanently.
func (repository *UserRepository) Delete(ctx context.Context, id string) (err error) {
	startedAt := time.Now()
	defer func() { repository.observe("delete_user", startedAt, err) }()
	return repository.queries.DeleteUser(ctx, id)
}

func rowToDomainUser(id, email, hash, role string, isActive bool, mustChangePassword bool, preferences []byte, createdAt, updatedAt, lastLoginAt pgtype.Timestamptz) domain.User {
	user := domain.User{
		ID:                 id,
		Email:              email,
		PasswordHash:       hash,
		Role:               domain.Role(role),
		IsActive:           isActive,
		MustChangePassword: mustChangePassword,
		Preferences:        unmarshalUserPreferences(preferences),
	}
	if createdAt.Valid {
		user.CreatedAt = createdAt.Time.UTC()
	}
	if updatedAt.Valid {
		user.UpdatedAt = updatedAt.Time.UTC()
	}
	if lastLoginAt.Valid {
		stamp := lastLoginAt.Time.UTC()
		user.LastLoginAt = &stamp
	}
	return user
}

func marshalUserPreferences(preferences domain.UserPreferences) []byte {
	payload, err := json.Marshal(domain.NormalizeUserPreferences(preferences))
	if err != nil {
		return []byte(`{}`)
	}
	return payload
}

func unmarshalUserPreferences(payload []byte) domain.UserPreferences {
	if len(payload) == 0 {
		return domain.NormalizeUserPreferences(domain.UserPreferences{})
	}
	var preferences domain.UserPreferences
	if err := json.Unmarshal(payload, &preferences); err != nil {
		return domain.NormalizeUserPreferences(domain.UserPreferences{})
	}
	return domain.NormalizeUserPreferences(preferences)
}
