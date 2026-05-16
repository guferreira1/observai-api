package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/core/ports"
	"github.com/guferreira1/observai-api/internal/platform/crypto"
)

// User is the admin-facing use case for managing application users.
type User struct {
	repository ports.UserRepository
	refresh    ports.RefreshTokenRepository
	ids        ports.IDGenerator
	now        func() time.Time
}

// NewUser creates a User administration use case.
//
// When refresh is non-nil, deactivating or deleting a user also revokes
// every outstanding refresh token belonging to them.
func NewUser(repository ports.UserRepository, refresh ports.RefreshTokenRepository, ids ports.IDGenerator) *User {
	return &User{repository: repository, refresh: refresh, ids: ids, now: time.Now}
}

// Create provisions a new user with the supplied role.
//
// The password is hashed with bcrypt before being persisted.
func (useCase *User) Create(ctx context.Context, email, password string, role domain.Role) (domain.User, error) {
	return useCase.CreateWithOptions(ctx, UserCreateRequest{
		Email:    email,
		Password: password,
		Role:     role,
	})
}

// UserCreateRequest carries admin-facing user provisioning options.
type UserCreateRequest struct {
	Name               string
	Email              string
	Password           string
	Role               domain.Role
	MustChangePassword bool
	Preferences        domain.UserPreferences
}

// CreateWithOptions provisions a user with optional security and preference flags.
func (useCase *User) CreateWithOptions(ctx context.Context, request UserCreateRequest) (domain.User, error) {
	cleanedEmail, err := normalizeEmail(request.Email)
	if err != nil {
		return domain.User{}, err
	}
	cleanedName := normalizeUserName(request.Name, cleanedEmail)
	if len(request.Password) < minPasswordLength {
		return domain.User{}, fmt.Errorf("%w: password must be at least %d characters", domain.ErrInvalidUser, minPasswordLength)
	}
	if !domain.IsValidRole(request.Role) {
		return domain.User{}, fmt.Errorf("%w: role %q is not supported", domain.ErrInvalidUser, request.Role)
	}
	id, err := useCase.ids.NextID(ctx)
	if err != nil {
		return domain.User{}, fmt.Errorf("generate user id: %w", err)
	}
	hash, err := crypto.HashPassword(request.Password, 0)
	if err != nil {
		return domain.User{}, fmt.Errorf("hash password: %w", err)
	}
	now := useCase.now().UTC()
	user := domain.User{
		ID:                 id,
		Name:               cleanedName,
		Email:              cleanedEmail,
		PasswordHash:       hash,
		Role:               request.Role,
		IsActive:           true,
		MustChangePassword: request.MustChangePassword,
		Preferences:        domain.NormalizeUserPreferences(request.Preferences),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := useCase.repository.Create(ctx, user); err != nil {
		return domain.User{}, err
	}
	return user, nil
}

func normalizeUserName(name string, email string) string {
	cleanedName := strings.TrimSpace(name)
	if cleanedName != "" {
		return cleanedName
	}
	if at := strings.IndexByte(email, '@'); at > 0 {
		return email[:at]
	}
	return email
}

// List returns persisted users, newest first.
func (useCase *User) List(ctx context.Context, limit int, offset int) ([]domain.User, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return useCase.repository.List(ctx, limit, offset)
}

// Get returns the user matching the supplied identifier.
func (useCase *User) Get(ctx context.Context, id string) (domain.User, error) {
	cleaned := strings.TrimSpace(id)
	if cleaned == "" {
		return domain.User{}, domain.ErrUserNotFound
	}
	return useCase.repository.FindByID(ctx, cleaned)
}

// UpdateRole replaces a user's role.
func (useCase *User) UpdateRole(ctx context.Context, id string, role domain.Role) (domain.User, error) {
	if !domain.IsValidRole(role) {
		return domain.User{}, fmt.Errorf("%w: role %q is not supported", domain.ErrInvalidUser, role)
	}
	now := useCase.now().UTC()
	if err := useCase.repository.UpdateRole(ctx, id, role, now); err != nil {
		return domain.User{}, err
	}
	return useCase.repository.FindByID(ctx, id)
}

// SetActive toggles a user's active flag. When deactivating, every active
// refresh token belonging to the user is revoked.
func (useCase *User) SetActive(ctx context.Context, id string, active bool) (domain.User, error) {
	now := useCase.now().UTC()
	if err := useCase.repository.SetActive(ctx, id, active, now); err != nil {
		return domain.User{}, err
	}
	if !active && useCase.refresh != nil {
		_ = useCase.refresh.RevokeAllForUser(ctx, id, now)
	}
	return useCase.repository.FindByID(ctx, id)
}

// SetMustChangePassword toggles the forced password-change flag.
func (useCase *User) SetMustChangePassword(ctx context.Context, id string, mustChangePassword bool) (domain.User, error) {
	now := useCase.now().UTC()
	if err := useCase.repository.SetMustChangePassword(ctx, id, mustChangePassword, now); err != nil {
		return domain.User{}, err
	}
	return useCase.repository.FindByID(ctx, id)
}

// Delete removes the user permanently and revokes their outstanding refresh
// tokens. Callers should typically prefer SetActive(false).
func (useCase *User) Delete(ctx context.Context, id string) error {
	cleaned := strings.TrimSpace(id)
	if cleaned == "" {
		return domain.ErrUserNotFound
	}
	if useCase.refresh != nil {
		_ = useCase.refresh.RevokeAllForUser(ctx, cleaned, useCase.now().UTC())
	}
	return useCase.repository.Delete(ctx, cleaned)
}
