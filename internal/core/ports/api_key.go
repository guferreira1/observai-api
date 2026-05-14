package ports

import (
	"context"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// APIKeyRepository persists API keys and provides authentication lookups.
//
// Implementations must store only the hashed value of the token; the raw
// secret is shown to the operator once at issue time.
type APIKeyRepository interface {
	Create(ctx context.Context, key domain.APIKey) error
	FindByHash(ctx context.Context, hash string) (domain.APIKey, error)
	List(ctx context.Context, limit int, offset int) ([]domain.APIKey, error)
	Revoke(ctx context.Context, id string) error
	TouchLastUsed(ctx context.Context, id string) error
}
