package testfakes

import (
	"context"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// ChatResponder answers analysis-scoped questions deterministically for tests.
type ChatResponder struct{}

// NewChatResponder creates a deterministic chat responder for tests.
func NewChatResponder() *ChatResponder {
	return &ChatResponder{}
}

// Answer returns a deterministic answer grounded in the active analysis.
//
// Evidence is returned as evidence identifiers so tests can assert citations
// against the stable IDs assigned by the analysis use case.
func (responder *ChatResponder) Answer(_ context.Context, analysis domain.AnalysisContext, question domain.ChatQuestion) (domain.ChatAnswer, error) {
	return domain.ChatAnswer{
		AnalysisID: question.AnalysisID,
		Answer:     "Based on the active analysis, focus on " + analysis.Summary,
		Evidence:   evidenceIdentifiers(analysis.Evidence),
	}, nil
}

func evidenceIdentifiers(evidence []domain.Evidence) []string {
	ids := make([]string, 0, len(evidence))
	for _, item := range evidence {
		ids = append(ids, item.ID)
	}
	return ids
}
