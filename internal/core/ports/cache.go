package ports

import (
	"context"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// AnalysisContextCache stores compact analysis context for scoped follow-up chat.
type AnalysisContextCache interface {
	Save(ctx context.Context, context domain.AnalysisContext, ttl time.Duration) error
	Find(ctx context.Context, analysisID string) (domain.AnalysisContext, error)
}
