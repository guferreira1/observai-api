package usecase

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

type stubAuditRepo struct {
	mu      sync.Mutex
	entries []domain.AuditEntry
}

func newStubAuditRepo() *stubAuditRepo {
	return &stubAuditRepo{entries: make([]domain.AuditEntry, 0)}
}

func (stub *stubAuditRepo) Append(_ context.Context, entry domain.AuditEntry) error {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	stub.entries = append(stub.entries, entry)
	return nil
}

func (stub *stubAuditRepo) List(_ context.Context, _ domain.AuditFilter) ([]domain.AuditEntry, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	out := make([]domain.AuditEntry, len(stub.entries))
	copy(out, stub.entries)
	return out, nil
}

func TestAuditAppendStampsTimestamp(t *testing.T) {
	repo := newStubAuditRepo()
	useCase := NewAuditLog(repo)
	frozen := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	useCase.now = func() time.Time { return frozen }

	if err := useCase.Append(context.Background(), domain.AuditEntry{Path: "/v1/ping"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	if len(repo.entries) != 1 || !repo.entries[0].CreatedAt.Equal(frozen) {
		t.Fatalf("timestamp not stamped: %+v", repo.entries[0])
	}
}

func TestAuditRecordPersistsGranularFields(t *testing.T) {
	repo := newStubAuditRepo()
	useCase := NewAuditLog(repo)

	if err := useCase.Record(context.Background(), AuditEvent{
		Actor:        "admin@observai.io",
		ActorID:      "user-1",
		Action:       "provider.created",
		ResourceType: "provider",
		ResourceID:   "prov-1",
		Metadata:     map[string]string{"type": "prometheus", "name": "prom"},
	}); err != nil {
		t.Fatalf("record: %v", err)
	}
	if len(repo.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(repo.entries))
	}
	entry := repo.entries[0]
	if entry.Action != "provider.created" || entry.ResourceID != "prov-1" || entry.APIKeyID != "user-1" {
		t.Fatalf("unexpected entry: %+v", entry)
	}
	if entry.Metadata["type"] != "prometheus" {
		t.Fatalf("metadata not persisted: %+v", entry.Metadata)
	}
}

func TestAuditRecordMasksSecretMetadata(t *testing.T) {
	repo := newStubAuditRepo()
	useCase := NewAuditLog(repo)

	if err := useCase.Record(context.Background(), AuditEvent{
		Action: "api_key.created",
		Metadata: map[string]string{
			"name":       "ci",
			"api_key":    "sk-supersecret-token",
			"password":   "h0rsebatterystaple",
			"token_hash": "abcd1234efgh5678",
		},
	}); err != nil {
		t.Fatalf("record: %v", err)
	}
	entry := repo.entries[0]
	if strings.Contains(entry.Metadata["api_key"], "supersecret") {
		t.Fatalf("api_key not masked: %q", entry.Metadata["api_key"])
	}
	if strings.Contains(entry.Metadata["password"], "h0rsebatterystaple") {
		t.Fatalf("password not masked: %q", entry.Metadata["password"])
	}
	if strings.Contains(entry.Metadata["token_hash"], "abcd1234") {
		t.Fatalf("token_hash not masked: %q", entry.Metadata["token_hash"])
	}
	if entry.Metadata["name"] != "ci" {
		t.Fatalf("non-secret key was masked: %q", entry.Metadata["name"])
	}
}
