package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/core/ports"
)

// Analysis orchestrates observability evidence collection and analysis generation.
type Analysis struct {
	collector  ports.SignalCollector
	generator  ports.AnalysisGenerator
	repository ports.AnalysisRepository
	cache      ports.AnalysisContextCache
	cacheTTL   time.Duration
	ids        ports.IDGenerator
	now        func() time.Time
}

// NewAnalysis creates an analysis use case.
func NewAnalysis(
	collector ports.SignalCollector,
	generator ports.AnalysisGenerator,
	repository ports.AnalysisRepository,
	cache ports.AnalysisContextCache,
	cacheTTL time.Duration,
	ids ports.IDGenerator,
) *Analysis {
	if cache == nil {
		cache = noOpAnalysisContextCache{}
	}

	return &Analysis{
		collector:  collector,
		generator:  generator,
		repository: repository,
		cache:      cache,
		cacheTTL:   cacheTTL,
		ids:        ids,
		now:        time.Now,
	}
}

// Analyze executes a provider-agnostic observability analysis.
func (useCase *Analysis) Analyze(ctx context.Context, request domain.AnalysisRequest) (domain.AnalysisResult, error) {
	if err := request.Validate(); err != nil {
		return domain.AnalysisResult{}, err
	}

	evidence, err := useCase.collector.Collect(ctx, request)
	if err != nil {
		return domain.AnalysisResult{}, fmt.Errorf("collect analysis evidence: %w", err)
	}

	result, err := useCase.generator.Generate(ctx, request, evidence)
	if err != nil {
		return domain.AnalysisResult{}, fmt.Errorf("generate analysis: %w", err)
	}

	id, err := useCase.ids.NextID(ctx)
	if err != nil {
		return domain.AnalysisResult{}, fmt.Errorf("create analysis id: %w", err)
	}

	result.ID = id
	result.Evidence = evidence
	result.CreatedAt = useCase.now().UTC()

	if err := useCase.repository.Save(ctx, result); err != nil {
		return domain.AnalysisResult{}, fmt.Errorf("save analysis: %w", err)
	}

	_ = useCase.cache.Save(ctx, domain.NewAnalysisContext(result), useCase.cacheTTL)

	return result, nil
}
