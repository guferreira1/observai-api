package null

import (
	"context"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// ChatResponder is a placeholder chat responder that fails every call.
type ChatResponder struct{}

// NewChatResponder builds a ChatResponder that returns ErrProviderNotConfigured.
func NewChatResponder() *ChatResponder {
	return &ChatResponder{}
}

// Answer rejects the question with domain.ErrProviderNotConfigured.
func (*ChatResponder) Answer(_ context.Context, _ domain.AnalysisContext, _ domain.ChatQuestion) (domain.ChatAnswer, error) {
	return domain.ChatAnswer{}, domain.ErrProviderNotConfigured
}
