package usecase

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

type stubAPIKeyRepository struct {
	mu   sync.Mutex
	keys map[string]domain.APIKey
}

func newStubAPIKeyRepository() *stubAPIKeyRepository {
	return &stubAPIKeyRepository{keys: make(map[string]domain.APIKey)}
}

func (stub *stubAPIKeyRepository) Create(_ context.Context, key domain.APIKey) error {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	stub.keys[key.Hash] = key
	return nil
}

func (stub *stubAPIKeyRepository) FindByHash(_ context.Context, hash string) (domain.APIKey, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	key, ok := stub.keys[hash]
	if !ok {
		return domain.APIKey{}, domain.ErrAPIKeyNotFound
	}
	return key, nil
}

func (stub *stubAPIKeyRepository) List(_ context.Context, _ int, _ int) ([]domain.APIKey, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	out := make([]domain.APIKey, 0, len(stub.keys))
	for _, key := range stub.keys {
		out = append(out, key)
	}
	return out, nil
}

func (stub *stubAPIKeyRepository) Revoke(_ context.Context, id string) error {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	for hash, key := range stub.keys {
		if key.ID != id || key.RevokedAt != nil {
			continue
		}
		now := time.Now().UTC()
		key.RevokedAt = &now
		stub.keys[hash] = key
		return nil
	}
	return nil
}

func (stub *stubAPIKeyRepository) TouchLastUsed(_ context.Context, id string) error {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	for hash, key := range stub.keys {
		if key.ID != id {
			continue
		}
		now := time.Now().UTC()
		key.LastUsedAt = &now
		stub.keys[hash] = key
		return nil
	}
	return nil
}

func newAPIKeyFixture() (*APIKey, *stubAPIKeyRepository) {
	repo := newStubAPIKeyRepository()
	return NewAPIKey(repo, &sequentialIDs{}), repo
}

func TestAPIKeyIssueRejectsMissingScopes(t *testing.T) {
	useCase, _ := newAPIKeyFixture()
	_, err := useCase.Issue(context.Background(), IssueAPIKeyRequest{Name: "ci"})
	if !errors.Is(err, domain.ErrInvalidAPIKey) {
		t.Fatalf("expected ErrInvalidAPIKey, got %v", err)
	}
}

func TestAPIKeyIssueRejectsUnknownScope(t *testing.T) {
	useCase, _ := newAPIKeyFixture()
	_, err := useCase.Issue(context.Background(), IssueAPIKeyRequest{
		Name:   "ci",
		Scopes: []domain.APIKeyScope{"bogus:scope"},
	})
	if !errors.Is(err, domain.ErrInvalidAPIKey) {
		t.Fatalf("expected ErrInvalidAPIKey, got %v", err)
	}
}

func TestAPIKeyIssueRejectsPastExpiration(t *testing.T) {
	useCase, _ := newAPIKeyFixture()
	past := time.Now().Add(-time.Hour)
	_, err := useCase.Issue(context.Background(), IssueAPIKeyRequest{
		Name:      "ci",
		Scopes:    []domain.APIKeyScope{domain.APIKeyScopeAnalysisRead},
		ExpiresAt: &past,
	})
	if !errors.Is(err, domain.ErrInvalidAPIKey) {
		t.Fatalf("expected ErrInvalidAPIKey for past expiration, got %v", err)
	}
}

func TestAPIKeyIssuePersistsAndReturnsSecret(t *testing.T) {
	useCase, repo := newAPIKeyFixture()
	issued, err := useCase.Issue(context.Background(), IssueAPIKeyRequest{
		Name:        "ci",
		Description: "deploy automation",
		Scopes:      []domain.APIKeyScope{domain.APIKeyScopeAnalysisWrite, domain.APIKeyScopeChatWrite},
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if issued.Secret == "" {
		t.Fatalf("expected plaintext secret returned at issue time")
	}
	if len(issued.APIKey.Scopes) != 2 {
		t.Fatalf("expected 2 scopes, got %v", issued.APIKey.Scopes)
	}
	if _, err := repo.FindByHash(context.Background(), issued.APIKey.Hash); err != nil {
		t.Fatalf("persisted key should be retrievable: %v", err)
	}
}

func TestAPIKeyResolveAcceptsValidToken(t *testing.T) {
	useCase, _ := newAPIKeyFixture()
	issued, _ := useCase.Issue(context.Background(), IssueAPIKeyRequest{
		Name:   "ci",
		Scopes: []domain.APIKeyScope{domain.APIKeyScopeAnalysisRead},
	})
	resolved, err := useCase.Resolve(context.Background(), issued.Secret)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !resolved.HasScope(domain.APIKeyScopeAnalysisRead) {
		t.Fatalf("expected analysis:read on resolved key, got %v", resolved.Scopes)
	}
}

func TestAPIKeyResolveRejectsRevokedToken(t *testing.T) {
	useCase, _ := newAPIKeyFixture()
	issued, _ := useCase.Issue(context.Background(), IssueAPIKeyRequest{
		Name:   "ci",
		Scopes: []domain.APIKeyScope{domain.APIKeyScopeAnalysisRead},
	})
	if err := useCase.Revoke(context.Background(), issued.APIKey.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := useCase.Resolve(context.Background(), issued.Secret); !errors.Is(err, domain.ErrAPIKeyRevoked) {
		t.Fatalf("expected ErrAPIKeyRevoked, got %v", err)
	}
}

func TestAPIKeyResolveRejectsExpiredToken(t *testing.T) {
	useCase, _ := newAPIKeyFixture()
	future := time.Now().Add(50 * time.Millisecond)
	issued, _ := useCase.Issue(context.Background(), IssueAPIKeyRequest{
		Name:      "ci",
		Scopes:    []domain.APIKeyScope{domain.APIKeyScopeAnalysisRead},
		ExpiresAt: &future,
	})
	useCase.now = func() time.Time { return future.Add(time.Second) }
	if _, err := useCase.Resolve(context.Background(), issued.Secret); !errors.Is(err, domain.ErrAPIKeyExpired) {
		t.Fatalf("expected ErrAPIKeyExpired, got %v", err)
	}
}

func TestAPIKeyResolveRejectsUnknownToken(t *testing.T) {
	useCase, _ := newAPIKeyFixture()
	if _, err := useCase.Resolve(context.Background(), "oai_unknown_token"); !errors.Is(err, domain.ErrAPIKeyNotFound) {
		t.Fatalf("expected ErrAPIKeyNotFound, got %v", err)
	}
}
