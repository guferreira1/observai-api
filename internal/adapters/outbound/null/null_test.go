package null

import (
	"context"
	"errors"
	"testing"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

func TestSignalCollectorReturnsProviderNotConfigured(t *testing.T) {
	evidence, err := NewSignalCollector().Collect(context.Background(), domain.AnalysisRequest{})
	if !errors.Is(err, domain.ErrProviderNotConfigured) {
		t.Fatalf("expected ErrProviderNotConfigured, got %v", err)
	}
	if evidence != nil {
		t.Fatalf("expected nil evidence, got %v", evidence)
	}
}

func TestAnalysisGeneratorReturnsProviderNotConfigured(t *testing.T) {
	result, err := NewAnalysisGenerator().Generate(context.Background(), domain.AnalysisRequest{}, nil)
	if !errors.Is(err, domain.ErrProviderNotConfigured) {
		t.Fatalf("expected ErrProviderNotConfigured, got %v", err)
	}
	if result.ID != "" || result.Summary != "" {
		t.Fatalf("expected zero-value result, got %+v", result)
	}
}

func TestChatResponderReturnsProviderNotConfigured(t *testing.T) {
	answer, err := NewChatResponder().Answer(context.Background(), domain.AnalysisContext{}, domain.ChatQuestion{})
	if !errors.Is(err, domain.ErrProviderNotConfigured) {
		t.Fatalf("expected ErrProviderNotConfigured, got %v", err)
	}
	if answer.AnalysisID != "" || answer.Answer != "" {
		t.Fatalf("expected zero-value answer, got %+v", answer)
	}
}

func TestTraceProviderReturnsProviderNotConfigured(t *testing.T) {
	spans, err := NewTraceProvider().FetchSpans(context.Background(), "anything")
	if !errors.Is(err, domain.ErrProviderNotConfigured) {
		t.Fatalf("expected ErrProviderNotConfigured, got %v", err)
	}
	if spans != nil {
		t.Fatalf("expected nil spans, got %v", spans)
	}
}
