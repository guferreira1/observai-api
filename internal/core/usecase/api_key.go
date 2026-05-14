package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/core/ports"
)

const apiKeySecretBytes = 32

// IssueAPIKeyRequest carries the fields accepted by APIKey.Issue.
type IssueAPIKeyRequest struct {
	Name        string
	Description string
	Scopes      []domain.APIKeyScope
	ExpiresAt   *time.Time
}

// APIKey is the use case responsible for issuing, listing and revoking
// persistent API keys.
type APIKey struct {
	repository ports.APIKeyRepository
	ids        ports.IDGenerator
	now        func() time.Time
}

// NewAPIKey creates an APIKey use case.
func NewAPIKey(repository ports.APIKeyRepository, ids ports.IDGenerator) *APIKey {
	return &APIKey{repository: repository, ids: ids, now: time.Now}
}

// Issue creates a new API key with the supplied request. The returned
// plaintext secret is the only opportunity the caller has to copy the key;
// the repository only ever stores the hash.
func (useCase *APIKey) Issue(ctx context.Context, request IssueAPIKeyRequest) (domain.IssuedAPIKey, error) {
	cleanedName := strings.TrimSpace(request.Name)
	if cleanedName == "" {
		return domain.IssuedAPIKey{}, fmt.Errorf("%w: name is required", domain.ErrInvalidAPIKey)
	}
	scopes, err := domain.NormalizeAPIKeyScopes(request.Scopes)
	if err != nil {
		return domain.IssuedAPIKey{}, err
	}
	if len(scopes) == 0 {
		return domain.IssuedAPIKey{}, fmt.Errorf("%w: at least one scope is required", domain.ErrInvalidAPIKey)
	}
	now := useCase.now().UTC()
	if request.ExpiresAt != nil && !request.ExpiresAt.After(now) {
		return domain.IssuedAPIKey{}, fmt.Errorf("%w: expiresAt must be in the future", domain.ErrInvalidAPIKey)
	}

	id, err := useCase.ids.NextID(ctx)
	if err != nil {
		return domain.IssuedAPIKey{}, fmt.Errorf("generate api key id: %w", err)
	}

	secret, err := generateAPIKeySecret(apiKeySecretBytes)
	if err != nil {
		return domain.IssuedAPIKey{}, fmt.Errorf("generate api key secret: %w", err)
	}

	key := domain.APIKey{
		ID:          id,
		Name:        cleanedName,
		Description: strings.TrimSpace(request.Description),
		Hash:        HashAPIKey(secret),
		Scopes:      scopes,
		CreatedAt:   now,
		ExpiresAt:   request.ExpiresAt,
	}
	if err := useCase.repository.Create(ctx, key); err != nil {
		return domain.IssuedAPIKey{}, fmt.Errorf("persist api key: %w", err)
	}

	return domain.IssuedAPIKey{APIKey: key, Secret: "oai_" + id + "_" + secret}, nil
}

// List returns persisted keys, newest first.
func (useCase *APIKey) List(ctx context.Context, limit int, offset int) ([]domain.APIKey, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return useCase.repository.List(ctx, limit, offset)
}

// Revoke marks the supplied key as revoked.
func (useCase *APIKey) Revoke(ctx context.Context, id string) error {
	cleaned := strings.TrimSpace(id)
	if cleaned == "" {
		return fmt.Errorf("%w: id is required", domain.ErrInvalidAPIKey)
	}
	return useCase.repository.Revoke(ctx, cleaned)
}

// Resolve looks up the API key matching the supplied plaintext token,
// records its usage and returns the domain representation. Returns
// ErrAPIKeyNotFound when the token is missing or unknown,
// ErrAPIKeyRevoked when the key has been revoked or ErrAPIKeyExpired
// when its expiry is in the past.
func (useCase *APIKey) Resolve(ctx context.Context, secret string) (domain.APIKey, error) {
	cleaned := strings.TrimSpace(secret)
	if cleaned == "" {
		return domain.APIKey{}, domain.ErrAPIKeyNotFound
	}

	hash := HashAPIKey(extractRawSecret(cleaned))
	key, err := useCase.repository.FindByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, domain.ErrAPIKeyNotFound) {
			return domain.APIKey{}, err
		}
		return domain.APIKey{}, fmt.Errorf("resolve api key: %w", err)
	}
	if key.IsRevoked() {
		return domain.APIKey{}, domain.ErrAPIKeyRevoked
	}
	if key.IsExpired(useCase.now().UTC()) {
		return domain.APIKey{}, domain.ErrAPIKeyExpired
	}
	_ = useCase.repository.TouchLastUsed(ctx, key.ID)
	return key, nil
}

// HashAPIKey returns the canonical hex-encoded SHA-256 digest used to
// persist API keys.
func HashAPIKey(secret string) string {
	digest := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(digest[:])
}

func extractRawSecret(token string) string {
	if strings.HasPrefix(token, "oai_") {
		parts := strings.SplitN(token, "_", 3)
		if len(parts) == 3 {
			return parts[2]
		}
	}
	return token
}

func generateAPIKeySecret(byteLen int) (string, error) {
	buffer := make([]byte, byteLen)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}
