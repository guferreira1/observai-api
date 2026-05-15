package usecase

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

type stubRetentionRepo struct {
	mu      sync.Mutex
	records map[string]time.Time
}

func newStubRetentionRepo() *stubRetentionRepo {
	return &stubRetentionRepo{records: make(map[string]time.Time)}
}

func (stub *stubRetentionRepo) DeleteByID(_ context.Context, id string) (int, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if _, ok := stub.records[id]; !ok {
		return 0, nil
	}
	delete(stub.records, id)
	return 1, nil
}

func (stub *stubRetentionRepo) DeleteOlderThan(_ context.Context, cutoff time.Time) (int, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	deleted := 0
	for id, created := range stub.records {
		if created.Before(cutoff) {
			delete(stub.records, id)
			deleted++
		}
	}
	return deleted, nil
}

func (stub *stubRetentionRepo) DeleteKeepingNewest(_ context.Context, keep int) (int, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if keep <= 0 || len(stub.records) <= keep {
		return 0, nil
	}
	type entry struct {
		id      string
		created time.Time
	}
	entries := make([]entry, 0, len(stub.records))
	for id, created := range stub.records {
		entries = append(entries, entry{id: id, created: created})
	}
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[j].created.After(entries[i].created) {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}
	deleted := 0
	for _, item := range entries[keep:] {
		delete(stub.records, item.id)
		deleted++
	}
	return deleted, nil
}

func TestRetentionDeleteReturnsNotFoundForUnknownID(t *testing.T) {
	useCase := NewAnalysisRetention(newStubRetentionRepo())
	err := useCase.Delete(context.Background(), "missing")
	if !errors.Is(err, domain.ErrAnalysisNotFound) {
		t.Fatalf("expected ErrAnalysisNotFound, got %v", err)
	}
}

func TestRetentionPurgeOnlyRemovesOldEnoughEntries(t *testing.T) {
	repo := newStubRetentionRepo()
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	repo.records["recent"] = now.Add(-30 * time.Minute)
	repo.records["old"] = now.Add(-48 * time.Hour)
	repo.records["older"] = now.Add(-72 * time.Hour)

	useCase := NewAnalysisRetention(repo)
	useCase.now = func() time.Time { return now }

	deleted, err := useCase.Purge(context.Background(), 24*time.Hour)
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("expected 2 purged, got %d", deleted)
	}
	if _, exists := repo.records["recent"]; !exists {
		t.Fatalf("recent record should have been preserved")
	}
}

func TestRetentionPurgeRejectsNonPositiveAge(t *testing.T) {
	useCase := NewAnalysisRetention(newStubRetentionRepo())
	if _, err := useCase.Purge(context.Background(), 0); err == nil {
		t.Fatalf("expected error for zero age")
	}
	if _, err := useCase.Purge(context.Background(), -time.Hour); err == nil {
		t.Fatalf("expected error for negative age")
	}
}

func TestRetentionPurgeByQuantityKeepsNewestN(t *testing.T) {
	repo := newStubRetentionRepo()
	base := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		repo.records[strconvI(i)] = base.Add(time.Duration(i) * time.Hour)
	}
	useCase := NewAnalysisRetention(repo)
	deleted, err := useCase.PurgeByQuantity(context.Background(), 2)
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if deleted != 3 {
		t.Fatalf("expected 3 deletions, got %d", deleted)
	}
	if len(repo.records) != 2 {
		t.Fatalf("expected 2 records preserved, got %d", len(repo.records))
	}
}

func TestRetentionPurgeByQuantityRejectsNonPositive(t *testing.T) {
	useCase := NewAnalysisRetention(newStubRetentionRepo())
	if _, err := useCase.PurgeByQuantity(context.Background(), 0); err == nil {
		t.Fatalf("expected error for zero keep")
	}
}

func strconvI(value int) string {
	return intToString(value)
}

func TestRetentionDeleteRemovesEntryFromRepository(t *testing.T) {
	repo := newStubRetentionRepo()
	repo.records["target"] = time.Now()

	useCase := NewAnalysisRetention(repo)
	if err := useCase.Delete(context.Background(), "target"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, exists := repo.records["target"]; exists {
		t.Fatalf("entry should have been removed")
	}
}
