package ports

import (
	"context"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// SignalCollector collects normalized observability evidence for an analysis request.
type SignalCollector interface {
	Collect(ctx context.Context, request domain.AnalysisRequest) ([]domain.Evidence, error)
}
