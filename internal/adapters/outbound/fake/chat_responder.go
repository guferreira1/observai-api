package fake

import (
	"context"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// ChatResponder answers analysis-scoped questions deterministically.
type ChatResponder struct{}

// NewChatResponder creates a fake chat responder.
func NewChatResponder() *ChatResponder {
	return &ChatResponder{}
}

// Answer returns a deterministic answer grounded in the active analysis.
func (responder *ChatResponder) Answer(_ context.Context, analysis domain.AnalysisContext, question domain.ChatQuestion) (domain.ChatAnswer, error) {
	return domain.ChatAnswer{
		AnalysisID: question.AnalysisID,
		Answer:     "Based on the active analysis, focus on " + analysis.Summary,
		Evidence:   evidenceNames(analysis.Evidence),
	}, nil
}
