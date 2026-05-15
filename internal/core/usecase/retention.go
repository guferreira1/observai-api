package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/core/ports"
)

// AnalysisRetention is the use case for deleting analyses individually or
// in bulk by age.
//
// The retention policy is hard-delete: the repository removes the
// analysis row immediately and the foreign keys on
// `analysis_chat_messages`, `chat_feedback`, `analysis_jobs` and webhook
// deliveries cascade so a single Delete/Purge invocation cleans up the
// entire object graph without leaving orphaned rows.
type AnalysisRetention struct {
	repository ports.AnalysisRetention
	now        func() time.Time
}

// NewAnalysisRetention creates a retention use case.
func NewAnalysisRetention(repository ports.AnalysisRetention) *AnalysisRetention {
	return &AnalysisRetention{repository: repository, now: time.Now}
}

// Delete removes a single analysis. Returns ErrAnalysisNotFound when the
// repository did not delete any row.
func (useCase *AnalysisRetention) Delete(ctx context.Context, id string) error {
	cleaned := strings.TrimSpace(id)
	if cleaned == "" {
		return fmt.Errorf("%w: analysis id is required", domain.ErrAnalysisNotFound)
	}
	affected, err := useCase.repository.DeleteByID(ctx, cleaned)
	if err != nil {
		return fmt.Errorf("delete analysis: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: %s", domain.ErrAnalysisNotFound, cleaned)
	}
	return nil
}

// Purge removes every analysis older than now-age. Age must be positive.
func (useCase *AnalysisRetention) Purge(ctx context.Context, age time.Duration) (int, error) {
	if age <= 0 {
		return 0, fmt.Errorf("retention age must be positive")
	}
	cutoff := useCase.now().UTC().Add(-age)
	affected, err := useCase.repository.DeleteOlderThan(ctx, cutoff)
	if err != nil {
		return 0, fmt.Errorf("purge analyses: %w", err)
	}
	return affected, nil
}

// PurgeByQuantity preserves the supplied number of newest analyses and
// removes the rest. The keep argument must be positive; the repository
// implements the truncation using `OFFSET keep` so the deletion is a
// single round-trip.
func (useCase *AnalysisRetention) PurgeByQuantity(ctx context.Context, keep int) (int, error) {
	if keep <= 0 {
		return 0, fmt.Errorf("retention quantity must be positive")
	}
	affected, err := useCase.repository.DeleteKeepingNewest(ctx, keep)
	if err != nil {
		return 0, fmt.Errorf("purge analyses keeping newest: %w", err)
	}
	return affected, nil
}
