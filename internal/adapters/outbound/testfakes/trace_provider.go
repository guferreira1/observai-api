package testfakes

import (
	"context"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// TraceProvider returns a deterministic four-span trace shaped like a typical
// checkout flow so router and use case tests can assert critical path,
// slowest spans and service dependency derivation.
type TraceProvider struct{}

// NewTraceProvider creates a deterministic trace provider for tests.
func NewTraceProvider() *TraceProvider {
	return &TraceProvider{}
}

// FetchSpans returns a deterministic trace. The analysisID is echoed into
// every span's TraceID so tests can correlate spans with the parent analysis.
func (provider *TraceProvider) FetchSpans(_ context.Context, analysisID string) ([]domain.Span, error) {
	traceID := analysisID
	if traceID == "" {
		traceID = "trace-test"
	}
	start := time.Unix(0, 0).UTC()
	return []domain.Span{
		{
			TraceID:    traceID,
			SpanID:     "span-root",
			Service:    "checkout-service",
			Operation:  "POST /checkout",
			StartTime:  start,
			DurationMs: 500,
			SelfTimeMs: 50,
			Status:     domain.SpanStatusOk,
		},
		{
			TraceID:      traceID,
			SpanID:       "span-payment",
			ParentSpanID: "span-root",
			Service:      "payment-service",
			Operation:    "authorize",
			StartTime:    start.Add(10 * time.Millisecond),
			DurationMs:   300,
			SelfTimeMs:   100,
			Status:       domain.SpanStatusOk,
		},
		{
			TraceID:      traceID,
			SpanID:       "span-db",
			ParentSpanID: "span-payment",
			Service:      "database",
			Operation:    "SELECT customers",
			StartTime:    start.Add(40 * time.Millisecond),
			DurationMs:   200,
			SelfTimeMs:   200,
			Status:       domain.SpanStatusOk,
		},
		{
			TraceID:      traceID,
			SpanID:       "span-external",
			ParentSpanID: "span-root",
			Service:      "external-api",
			Operation:    "GET /risk",
			StartTime:    start.Add(20 * time.Millisecond),
			DurationMs:   150,
			SelfTimeMs:   150,
			Status:       domain.SpanStatusError,
		},
	}, nil
}
