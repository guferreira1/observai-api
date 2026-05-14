package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/core/ports"
)

// AuditLog is the use case for persisting and listing audit entries.
type AuditLog struct {
	repository ports.AuditLogRepository
	now        func() time.Time
}

// NewAuditLog creates an AuditLog use case.
func NewAuditLog(repository ports.AuditLogRepository) *AuditLog {
	return &AuditLog{repository: repository, now: time.Now}
}

// Append persists a single audit entry. The timestamp is overridden with
// the use case clock when the caller did not provide one.
func (useCase *AuditLog) Append(ctx context.Context, entry domain.AuditEntry) error {
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = useCase.now().UTC()
	}
	if err := useCase.repository.Append(ctx, entry); err != nil {
		return fmt.Errorf("append audit entry: %w", err)
	}
	return nil
}

// List returns the most recent audit entries honoring the filter.
func (useCase *AuditLog) List(ctx context.Context, filter domain.AuditFilter) ([]domain.AuditEntry, error) {
	return useCase.repository.List(ctx, filter)
}
