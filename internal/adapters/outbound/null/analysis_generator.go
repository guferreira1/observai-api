package null

import (
	"context"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// AnalysisGenerator is a placeholder LLM generator that fails every call.
type AnalysisGenerator struct{}

// NewAnalysisGenerator builds an AnalysisGenerator that returns ErrProviderNotConfigured.
func NewAnalysisGenerator() *AnalysisGenerator {
	return &AnalysisGenerator{}
}

// Generate rejects the request with domain.ErrProviderNotConfigured.
func (*AnalysisGenerator) Generate(_ context.Context, _ domain.AnalysisRequest, _ []domain.Evidence) (domain.AnalysisResult, error) {
	return domain.AnalysisResult{}, domain.ErrProviderNotConfigured
}
