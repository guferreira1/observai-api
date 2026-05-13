package ports

import (
	"context"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// AnalysisRepository stores and retrieves analysis results.
type AnalysisRepository interface {
	Save(ctx context.Context, result domain.AnalysisResult) error
	Find(ctx context.Context, id string) (domain.AnalysisResult, error)
	ListAnalyses(ctx context.Context, filter domain.AnalysisListFilter) (domain.AnalysisList, error)
}

// IDGenerator creates identifiers for domain resources.
type IDGenerator interface {
	NextID(ctx context.Context) (string, error)
}
