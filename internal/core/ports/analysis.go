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

// AnalysisStatsRepository computes aggregated statistics over stored analyses.
//
// Adapters that can push aggregation down to the storage backend (postgres
// histograms, materialized views) implement this port. The use case checks
// for the implementation and falls back to in-process computation when the
// repository in use does not implement it.
type AnalysisStatsRepository interface {
	AnalysisStats(ctx context.Context, filter domain.AnalysisStatsFilter, topServiceCount int) (domain.AnalysisStats, error)
}

// AnalysisServiceCatalog returns unique service names derived from stored analyses.
//
// Adapters that can answer this query in SQL (postgres `unnest(affected_services)`)
// implement this port; otherwise the use case falls back to listing analyses
// and deduplicating service names in memory.
type AnalysisServiceCatalog interface {
	ListAffectedServices(ctx context.Context, query string, limit int) ([]string, error)
}

// IDGenerator creates identifiers for domain resources.
type IDGenerator interface {
	NextID(ctx context.Context) (string, error)
}
