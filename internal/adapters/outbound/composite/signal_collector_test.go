package composite

import (
	"context"
	"errors"
	"testing"

	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/core/policy"
)

type stubCollector struct {
	evidence []domain.Evidence
	err      error
}

func (collector stubCollector) Collect(_ context.Context, _ domain.AnalysisRequest) ([]domain.Evidence, error) {
	return collector.evidence, collector.err
}

func TestCompositeAggregatesEvidenceInDeclarationOrder(t *testing.T) {
	composite := NewSignalCollector([]NamedCollector{
		{Name: "prometheus", Collector: stubCollector{evidence: []domain.Evidence{{Name: "p95_latency"}}}},
		{Name: "loki", Collector: stubCollector{evidence: []domain.Evidence{{Name: "error_log"}}}},
	}, Options{})

	evidence, err := composite.Collect(context.Background(), domain.AnalysisRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(evidence) != 2 {
		t.Fatalf("expected 2 evidence items, got %d", len(evidence))
	}
	if evidence[0].Name != "p95_latency" || evidence[0].Provider != "prometheus" {
		t.Fatalf("first evidence has wrong content: %+v", evidence[0])
	}
	if evidence[1].Name != "error_log" || evidence[1].Provider != "loki" {
		t.Fatalf("second evidence has wrong content: %+v", evidence[1])
	}
}

func TestCompositePartialFailureSkipsFailingCollector(t *testing.T) {
	composite := NewSignalCollector([]NamedCollector{
		{Name: "prometheus", Collector: stubCollector{err: errors.New("boom")}},
		{Name: "loki", Collector: stubCollector{evidence: []domain.Evidence{{Name: "ok"}}}},
	}, Options{ErrorPolicy: policy.NewPartialFailureCollectorErrorPolicy()})

	evidence, err := composite.Collect(context.Background(), domain.AnalysisRequest{})
	if err != nil {
		t.Fatalf("unexpected error from partial-failure policy: %v", err)
	}
	if len(evidence) != 1 {
		t.Fatalf("expected 1 evidence item after partial failure, got %d", len(evidence))
	}
}

func TestCompositeFailFastAbortsOnFirstError(t *testing.T) {
	composite := NewSignalCollector([]NamedCollector{
		{Name: "prometheus", Collector: stubCollector{err: errors.New("boom")}},
		{Name: "loki", Collector: stubCollector{evidence: []domain.Evidence{{Name: "ok"}}}},
	}, Options{ErrorPolicy: policy.NewFailFastCollectorErrorPolicy()})

	_, err := composite.Collect(context.Background(), domain.AnalysisRequest{})
	if err == nil {
		t.Fatal("expected error from fail-fast policy")
	}
}

func TestCompositeReturnsErrorWhenNoCollectorsRegistered(t *testing.T) {
	composite := NewSignalCollector(nil, Options{})
	_, err := composite.Collect(context.Background(), domain.AnalysisRequest{})
	if err == nil {
		t.Fatal("expected error when no collectors are registered")
	}
}

func TestCompositePreservesExistingProviderAttribution(t *testing.T) {
	composite := NewSignalCollector([]NamedCollector{
		{Name: "wrapper", Collector: stubCollector{evidence: []domain.Evidence{{Name: "x", Provider: "underlying"}}}},
	}, Options{})

	evidence, err := composite.Collect(context.Background(), domain.AnalysisRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evidence[0].Provider != "underlying" {
		t.Fatalf("composite must not overwrite explicit provider attribution, got %q", evidence[0].Provider)
	}
}
