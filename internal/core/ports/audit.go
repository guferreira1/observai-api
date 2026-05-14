package ports

import (
	"context"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// AuditLogRepository persists and lists authenticated request entries.
type AuditLogRepository interface {
	Append(ctx context.Context, entry domain.AuditEntry) error
	List(ctx context.Context, filter domain.AuditFilter) ([]domain.AuditEntry, error)
}
