package ports

import (
	"context"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// UserRepository persists application users.
type UserRepository interface {
	Create(ctx context.Context, user domain.User) error
	FindByID(ctx context.Context, id string) (domain.User, error)
	FindByEmail(ctx context.Context, email string) (domain.User, error)
	List(ctx context.Context, limit int, offset int) ([]domain.User, error)
	Count(ctx context.Context) (int64, error)
	UpdateProfile(ctx context.Context, id string, name string, email string, updatedAt time.Time) error
	UpdatePreferences(ctx context.Context, id string, preferences domain.UserPreferences, updatedAt time.Time) error
	UpdatePassword(ctx context.Context, id string, passwordHash string, updatedAt time.Time) error
	UpdateRole(ctx context.Context, id string, role domain.Role, updatedAt time.Time) error
	SetActive(ctx context.Context, id string, active bool, updatedAt time.Time) error
	SetMustChangePassword(ctx context.Context, id string, mustChangePassword bool, updatedAt time.Time) error
	TouchLastLogin(ctx context.Context, id string, when time.Time) error
	Delete(ctx context.Context, id string) error
}

// RefreshTokenRepository persists refresh tokens with rotation support.
type RefreshTokenRepository interface {
	Create(ctx context.Context, token domain.RefreshToken) error
	FindByHash(ctx context.Context, hash string) (domain.RefreshToken, error)
	Revoke(ctx context.Context, id string, replacedBy *string, when time.Time) error
	RevokeFamily(ctx context.Context, familyID string, when time.Time) error
	RevokeAllForUser(ctx context.Context, userID string, when time.Time) error
}
