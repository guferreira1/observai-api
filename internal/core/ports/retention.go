package ports

import (
	"context"
	"time"
)

// AnalysisRetention exposes destructive operations against the analysis
// store. Implementations cascade related rows (chat history, feedback,
// jobs) through database foreign keys.
type AnalysisRetention interface {
	DeleteByID(ctx context.Context, id string) (int, error)
	CountOlderThan(ctx context.Context, cutoff time.Time) (int, error)
	DeleteOlderThan(ctx context.Context, cutoff time.Time) (int, error)
	CountExceedingNewest(ctx context.Context, keep int) (int, error)
	DeleteKeepingNewest(ctx context.Context, keep int) (int, error)
}
