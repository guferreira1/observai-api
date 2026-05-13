package ports

import (
	"context"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// AnalysisGenerator converts normalized evidence into an analysis result.
type AnalysisGenerator interface {
	Generate(ctx context.Context, request domain.AnalysisRequest, evidence []domain.Evidence) (domain.AnalysisResult, error)
}

// ChatResponder answers scoped questions about an analysis result.
type ChatResponder interface {
	Answer(ctx context.Context, analysis domain.AnalysisContext, question domain.ChatQuestion) (domain.ChatAnswer, error)
}
