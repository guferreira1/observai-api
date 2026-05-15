package inmemory

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// UserRepository stores users in memory for local development without Postgres.
type UserRepository struct {
	mu    sync.RWMutex
	users map[string]domain.User
}

// NewUserRepository creates an in-memory user repository.
func NewUserRepository() *UserRepository {
	return &UserRepository{users: make(map[string]domain.User)}
}

// Create stores a new user. Emails are matched case-insensitively.
func (repository *UserRepository) Create(_ context.Context, user domain.User) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	normalized := strings.ToLower(strings.TrimSpace(user.Email))
	for _, existing := range repository.users {
		if strings.EqualFold(existing.Email, normalized) {
			return domain.ErrUserAlreadyExists
		}
	}
	user.Email = normalized
	user.Preferences = domain.NormalizeUserPreferences(user.Preferences)
	repository.users[user.ID] = user
	return nil
}

// FindByID returns the user matching the supplied identifier.
func (repository *UserRepository) FindByID(_ context.Context, id string) (domain.User, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	user, ok := repository.users[id]
	if !ok {
		return domain.User{}, domain.ErrUserNotFound
	}
	return user, nil
}

// FindByEmail returns the user matching the supplied email.
func (repository *UserRepository) FindByEmail(_ context.Context, email string) (domain.User, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	normalized := strings.ToLower(strings.TrimSpace(email))
	for _, user := range repository.users {
		if strings.EqualFold(user.Email, normalized) {
			return user, nil
		}
	}
	return domain.User{}, domain.ErrUserNotFound
}

// List returns persisted users ordered by creation time, newest first.
func (repository *UserRepository) List(_ context.Context, limit int, offset int) ([]domain.User, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	out := make([]domain.User, 0, len(repository.users))
	for _, user := range repository.users {
		out = append(out, user)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if offset >= len(out) {
		return []domain.User{}, nil
	}
	end := offset + limit
	if limit <= 0 || end > len(out) {
		end = len(out)
	}
	return out[offset:end], nil
}

// Count returns the total number of users.
func (repository *UserRepository) Count(_ context.Context) (int64, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	return int64(len(repository.users)), nil
}

// UpdateProfile updates the email and updatedAt timestamp.
func (repository *UserRepository) UpdateProfile(_ context.Context, id string, email string, updatedAt time.Time) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	user, ok := repository.users[id]
	if !ok {
		return domain.ErrUserNotFound
	}
	normalized := strings.ToLower(strings.TrimSpace(email))
	for otherID, other := range repository.users {
		if otherID == id {
			continue
		}
		if strings.EqualFold(other.Email, normalized) {
			return domain.ErrUserAlreadyExists
		}
	}
	user.Email = normalized
	user.UpdatedAt = updatedAt
	repository.users[id] = user
	return nil
}

// UpdatePreferences stores the user's UI preference projection.
func (repository *UserRepository) UpdatePreferences(_ context.Context, id string, preferences domain.UserPreferences, updatedAt time.Time) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	user, ok := repository.users[id]
	if !ok {
		return domain.ErrUserNotFound
	}
	user.Preferences = domain.NormalizeUserPreferences(preferences)
	user.UpdatedAt = updatedAt
	repository.users[id] = user
	return nil
}

// UpdatePassword replaces the user's password hash.
func (repository *UserRepository) UpdatePassword(_ context.Context, id string, passwordHash string, updatedAt time.Time) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	user, ok := repository.users[id]
	if !ok {
		return domain.ErrUserNotFound
	}
	user.PasswordHash = passwordHash
	user.MustChangePassword = false
	user.UpdatedAt = updatedAt
	repository.users[id] = user
	return nil
}

// UpdateRole replaces the user's role.
func (repository *UserRepository) UpdateRole(_ context.Context, id string, role domain.Role, updatedAt time.Time) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	user, ok := repository.users[id]
	if !ok {
		return domain.ErrUserNotFound
	}
	user.Role = role
	user.UpdatedAt = updatedAt
	repository.users[id] = user
	return nil
}

// SetActive toggles the user's active flag.
func (repository *UserRepository) SetActive(_ context.Context, id string, active bool, updatedAt time.Time) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	user, ok := repository.users[id]
	if !ok {
		return domain.ErrUserNotFound
	}
	user.IsActive = active
	user.UpdatedAt = updatedAt
	repository.users[id] = user
	return nil
}

// SetMustChangePassword toggles the forced password-change flag.
func (repository *UserRepository) SetMustChangePassword(_ context.Context, id string, mustChangePassword bool, updatedAt time.Time) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	user, ok := repository.users[id]
	if !ok {
		return domain.ErrUserNotFound
	}
	user.MustChangePassword = mustChangePassword
	user.UpdatedAt = updatedAt
	repository.users[id] = user
	return nil
}

// TouchLastLogin records the last successful login timestamp.
func (repository *UserRepository) TouchLastLogin(_ context.Context, id string, when time.Time) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	user, ok := repository.users[id]
	if !ok {
		return domain.ErrUserNotFound
	}
	stamp := when.UTC()
	user.LastLoginAt = &stamp
	user.UpdatedAt = when.UTC()
	repository.users[id] = user
	return nil
}

// Delete removes the user permanently.
func (repository *UserRepository) Delete(_ context.Context, id string) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	delete(repository.users, id)
	return nil
}
