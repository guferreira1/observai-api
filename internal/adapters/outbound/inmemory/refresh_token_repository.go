package inmemory

import (
	"context"
	"sync"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// RefreshTokenRepository stores refresh tokens in memory.
type RefreshTokenRepository struct {
	mu     sync.RWMutex
	tokens map[string]domain.RefreshToken
}

// NewRefreshTokenRepository creates an in-memory refresh-token repository.
func NewRefreshTokenRepository() *RefreshTokenRepository {
	return &RefreshTokenRepository{tokens: make(map[string]domain.RefreshToken)}
}

// Create persists a new refresh token, indexed by hash.
func (repository *RefreshTokenRepository) Create(_ context.Context, token domain.RefreshToken) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.tokens[token.TokenHash] = token
	return nil
}

// FindByHash returns the token matching the supplied hash, including revoked
// or expired entries so callers can detect reuse.
func (repository *RefreshTokenRepository) FindByHash(_ context.Context, hash string) (domain.RefreshToken, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	token, ok := repository.tokens[hash]
	if !ok {
		return domain.RefreshToken{}, domain.ErrInvalidRefreshToken
	}
	return token, nil
}

// Revoke marks the supplied token as revoked.
func (repository *RefreshTokenRepository) Revoke(_ context.Context, id string, replacedBy *string, when time.Time) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	for hash, token := range repository.tokens {
		if token.ID != id || token.RevokedAt != nil {
			continue
		}
		stamp := when.UTC()
		token.RevokedAt = &stamp
		token.ReplacedBy = replacedBy
		repository.tokens[hash] = token
		return nil
	}
	return nil
}

// RevokeFamily marks every active token in a family as revoked.
func (repository *RefreshTokenRepository) RevokeFamily(_ context.Context, familyID string, when time.Time) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	stamp := when.UTC()
	for hash, token := range repository.tokens {
		if token.FamilyID != familyID || token.RevokedAt != nil {
			continue
		}
		token.RevokedAt = &stamp
		repository.tokens[hash] = token
	}
	return nil
}

// RevokeAllForUser revokes every active token belonging to the supplied user.
func (repository *RefreshTokenRepository) RevokeAllForUser(_ context.Context, userID string, when time.Time) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	stamp := when.UTC()
	for hash, token := range repository.tokens {
		if token.UserID != userID || token.RevokedAt != nil {
			continue
		}
		token.RevokedAt = &stamp
		repository.tokens[hash] = token
	}
	return nil
}
