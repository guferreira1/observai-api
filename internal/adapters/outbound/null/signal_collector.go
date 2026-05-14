// Package null provides outbound adapters that refuse to serve requests with
// a sentinel error instead of returning synthetic data.
//
// These adapters are wired in local/dev mode when the operator has not
// configured a real observability or LLM provider. They prevent the API from
// silently fabricating evidence or analyses, which would mislead callers and
// poison stored history.
package null

import (
	"context"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// SignalCollector is a placeholder signal collector that fails every call.
type SignalCollector struct{}

// NewSignalCollector builds a SignalCollector that returns ErrProviderNotConfigured.
func NewSignalCollector() *SignalCollector {
	return &SignalCollector{}
}

// Collect rejects the request with domain.ErrProviderNotConfigured.
func (*SignalCollector) Collect(_ context.Context, _ domain.AnalysisRequest) ([]domain.Evidence, error) {
	return nil, domain.ErrProviderNotConfigured
}
