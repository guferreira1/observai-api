package ports

import (
	"context"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// TraceProvider returns provider-agnostic spans for an analysis.
//
// Adapters implement this port for each tracing backend (Jaeger, Tempo, OTLP)
// and must translate provider-specific payloads into [domain.Span] before
// returning. The use case derives critical path, slowest spans and service
// dependencies from the returned spans.
type TraceProvider interface {
	FetchSpans(ctx context.Context, analysisID string) ([]domain.Span, error)
}
