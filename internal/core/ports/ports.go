package ports

import (
	"context"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// SignalCollector collects normalized observability evidence for an analysis request.
type SignalCollector interface {
	Collect(ctx context.Context, request domain.AnalysisRequest) ([]domain.Evidence, error)
}

// AnalysisGenerator converts normalized evidence into an analysis result.
type AnalysisGenerator interface {
	Generate(ctx context.Context, request domain.AnalysisRequest, evidence []domain.Evidence) (domain.AnalysisResult, error)
}

// AnalysisRepository stores and retrieves analysis results.
type AnalysisRepository interface {
	Save(ctx context.Context, result domain.AnalysisResult) error
	Find(ctx context.Context, id string) (domain.AnalysisResult, error)
}

// AnalysisContextCache stores compact analysis context for scoped follow-up chat.
type AnalysisContextCache interface {
	Save(ctx context.Context, context domain.AnalysisContext, ttl time.Duration) error
	Find(ctx context.Context, analysisID string) (domain.AnalysisContext, error)
}

// ChatHistoryRepository stores and retrieves persistent analysis chat history.
type ChatHistoryRepository interface {
	SaveExchange(ctx context.Context, question domain.ChatMessage, answer domain.ChatMessage) error
	List(ctx context.Context, analysisID string) ([]domain.ChatMessage, error)
}

// ChatResponder answers scoped questions about an analysis result.
type ChatResponder interface {
	Answer(ctx context.Context, analysis domain.AnalysisContext, question domain.ChatQuestion) (domain.ChatAnswer, error)
}

// IDGenerator creates identifiers for domain resources.
type IDGenerator interface {
	NextID(ctx context.Context) (string, error)
}
