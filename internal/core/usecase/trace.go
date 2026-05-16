package usecase

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/core/ports"
)

const defaultSlowestSpanCount = 5

// Trace orchestrates trace retrieval and structural insight derivation.
type Trace struct {
	repository   ports.AnalysisRepository
	traces       ports.TraceProvider
	insightRules []TraceInsightRule
}

// TraceInsightRule populates a derived field on the resulting trace insights bundle.
//
// Rules are independently testable, receive the already-fetched spans and the
// in-progress insights so they can compose without coupling to each other.
type TraceInsightRule interface {
	Apply(spans []domain.Span, insights *domain.TraceInsights)
}

// NewTrace creates a trace use case.
//
// The repository is consulted only to confirm the analysis exists; spans
// always come from the TraceProvider port. Default rules compute slowest
// spans, critical path and service dependencies; callers may replace them
// with WithInsightRules.
func NewTrace(repository ports.AnalysisRepository, traces ports.TraceProvider) *Trace {
	return &Trace{
		repository: repository,
		traces:     traces,
		insightRules: []TraceInsightRule{
			SlowestSpansRule{Top: defaultSlowestSpanCount},
			CriticalPathRule{},
			DependencyEdgesRule{},
		},
	}
}

// WithInsightRules replaces the default rules. Useful for tests and for opting
// out of expensive insights in environments that cannot honor them.
func (useCase *Trace) WithInsightRules(rules ...TraceInsightRule) *Trace {
	useCase.insightRules = rules
	return useCase
}

// Get returns the structured trace insights for an analysis.
func (useCase *Trace) Get(ctx context.Context, analysisID string) (domain.TraceInsights, error) {
	analysisID = strings.TrimSpace(analysisID)
	if analysisID == "" {
		return domain.TraceInsights{}, fmt.Errorf("%w: analysis id is required", domain.ErrAnalysisNotFound)
	}

	analysis, err := useCase.repository.Find(ctx, analysisID)
	if err != nil {
		return domain.TraceInsights{}, fmt.Errorf("find analysis: %w", err)
	}

	if useCase.traces == nil {
		return domain.TraceInsights{}, errors.New("trace provider not configured")
	}

	traceReference := strings.TrimSpace(analysis.TraceID)
	if traceReference == "" {
		return domain.TraceInsights{}, fmt.Errorf("%w: analysis does not contain a trace id", domain.ErrTraceNotFound)
	}

	spans, err := useCase.traces.FetchSpans(ctx, traceReference)
	if err != nil {
		return domain.TraceInsights{}, fmt.Errorf("fetch trace spans: %w", err)
	}

	insights := domain.TraceInsights{Spans: spans}
	for _, rule := range useCase.insightRules {
		rule.Apply(spans, &insights)
	}
	return insights, nil
}

// SlowestSpansRule populates the top-N slowest spans by duration.
type SlowestSpansRule struct {
	Top int
}

// Apply implements TraceInsightRule.
func (rule SlowestSpansRule) Apply(spans []domain.Span, insights *domain.TraceInsights) {
	if len(spans) == 0 {
		insights.SlowestSpanIDs = []string{}
		return
	}

	indexed := make([]domain.Span, len(spans))
	copy(indexed, spans)
	sort.SliceStable(indexed, func(left, right int) bool {
		return indexed[left].DurationMs > indexed[right].DurationMs
	})

	limit := rule.Top
	if limit <= 0 || limit > len(indexed) {
		limit = len(indexed)
	}

	ids := make([]string, 0, limit)
	for index := 0; index < limit; index++ {
		ids = append(ids, indexed[index].SpanID)
	}
	insights.SlowestSpanIDs = ids
}

// CriticalPathRule walks the longest parent/child chain by duration.
//
// The root is the longest span without a parent (or the overall longest if
// multiple roots exist). From each parent the rule descends into the child
// that contributes the largest fraction of the parent's duration.
type CriticalPathRule struct{}

// Apply implements TraceInsightRule.
func (rule CriticalPathRule) Apply(spans []domain.Span, insights *domain.TraceInsights) {
	if len(spans) == 0 {
		insights.CriticalPathSpanIDs = []string{}
		return
	}

	bySpanID := make(map[string]domain.Span, len(spans))
	children := make(map[string][]domain.Span, len(spans))
	for _, span := range spans {
		bySpanID[span.SpanID] = span
		children[span.ParentSpanID] = append(children[span.ParentSpanID], span)
	}

	root := pickRoot(spans, children)
	if root.SpanID == "" {
		insights.CriticalPathSpanIDs = []string{}
		return
	}

	path := []string{root.SpanID}
	current := root
	for {
		next, ok := slowestChild(children[current.SpanID])
		if !ok {
			break
		}
		path = append(path, next.SpanID)
		current = next
	}
	insights.CriticalPathSpanIDs = path
}

func pickRoot(spans []domain.Span, children map[string][]domain.Span) domain.Span {
	var root domain.Span
	maxDuration := -1.0
	for _, span := range spans {
		if span.ParentSpanID != "" {
			continue
		}
		if span.DurationMs > maxDuration {
			maxDuration = span.DurationMs
			root = span
		}
	}
	if root.SpanID != "" {
		return root
	}
	for _, span := range spans {
		if span.DurationMs > maxDuration {
			maxDuration = span.DurationMs
			root = span
		}
	}
	return root
}

func slowestChild(children []domain.Span) (domain.Span, bool) {
	if len(children) == 0 {
		return domain.Span{}, false
	}
	slowest := children[0]
	for _, child := range children[1:] {
		if child.DurationMs > slowest.DurationMs {
			slowest = child
		}
	}
	return slowest, true
}

// DependencyEdgesRule aggregates parent->child service edges.
type DependencyEdgesRule struct{}

// Apply implements TraceInsightRule.
func (rule DependencyEdgesRule) Apply(spans []domain.Span, insights *domain.TraceInsights) {
	if len(spans) == 0 {
		insights.DependencyEdges = []domain.TraceDependencyEdge{}
		return
	}

	bySpanID := make(map[string]domain.Span, len(spans))
	for _, span := range spans {
		bySpanID[span.SpanID] = span
	}

	type edgeKey struct {
		from string
		to   string
	}
	durations := make(map[edgeKey][]float64)
	for _, span := range spans {
		parent, ok := bySpanID[span.ParentSpanID]
		if !ok || parent.Service == "" || span.Service == "" || parent.Service == span.Service {
			continue
		}
		key := edgeKey{from: parent.Service, to: span.Service}
		durations[key] = append(durations[key], span.DurationMs)
	}

	edges := make([]domain.TraceDependencyEdge, 0, len(durations))
	for key, list := range durations {
		edges = append(edges, domain.TraceDependencyEdge{
			From:      key.from,
			To:        key.to,
			CallCount: len(list),
			P95Ms:     percentile(list, 95),
		})
	}
	sort.SliceStable(edges, func(left, right int) bool {
		if edges[left].From == edges[right].From {
			return edges[left].To < edges[right].To
		}
		return edges[left].From < edges[right].From
	})
	insights.DependencyEdges = edges
}

func percentile(values []float64, pct int) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)
	index := (pct * (len(sorted) - 1)) / 100
	return sorted[index]
}
