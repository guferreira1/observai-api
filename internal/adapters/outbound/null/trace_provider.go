package null

import (
	"context"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// TraceProvider is a placeholder trace provider that fails every call.
type TraceProvider struct{}

// NewTraceProvider builds a TraceProvider that returns ErrProviderNotConfigured.
func NewTraceProvider() *TraceProvider {
	return &TraceProvider{}
}

// FetchSpans rejects the request with domain.ErrProviderNotConfigured.
func (*TraceProvider) FetchSpans(_ context.Context, _ string) ([]domain.Span, error) {
	return nil, domain.ErrProviderNotConfigured
}
