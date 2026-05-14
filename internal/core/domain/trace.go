package domain

import "time"

// Span describes a single, normalized span returned to API clients.
//
// Adapters translate provider-specific span payloads (Jaeger, Tempo, OTLP)
// into this provider-agnostic shape before reaching the use case.
type Span struct {
	TraceID      string
	SpanID       string
	ParentSpanID string
	Service      string
	Operation    string
	StartTime    time.Time
	DurationMs   float64
	SelfTimeMs   float64
	Status       SpanStatus
	Attributes   map[string]string
}

// SpanStatus identifies the public outcome of a span.
type SpanStatus string

const (
	// SpanStatusOk indicates the span completed without error.
	SpanStatusOk SpanStatus = "ok"
	// SpanStatusError indicates the span completed with an error.
	SpanStatusError SpanStatus = "error"
	// SpanStatusUnset indicates the provider did not report a status.
	SpanStatusUnset SpanStatus = "unset"
)

// TraceDependencyEdge describes an aggregated dependency between two services.
type TraceDependencyEdge struct {
	From      string
	To        string
	CallCount int
	P95Ms     float64
}

// TraceInsights bundles the structured traces and derived insights for an analysis.
type TraceInsights struct {
	Spans               []Span
	CriticalPathSpanIDs []string
	SlowestSpanIDs      []string
	DependencyEdges     []TraceDependencyEdge
}
