package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/core/policy"
	"github.com/guferreira1/observai-api/internal/core/ports"
)

const (
	defaultAnalysisListLimit = 20
	maxAnalysisListLimit     = 100
	maxLLMEvidenceItems      = 25
)

// Analysis orchestrates observability evidence collection and analysis generation.
type Analysis struct {
	collector       ports.SignalCollector
	generator       ports.AnalysisGenerator
	repository      ports.AnalysisRepository
	cache           ports.AnalysisContextCache
	cacheTTL        time.Duration
	ids             ports.IDGenerator
	severity        policy.SeverityPolicy
	recommendations policy.RecommendationPolicy
	now             func() time.Time
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
		collector:       collector,
		generator:       generator,
		repository:      repository,
		cache:           cache,
		cacheTTL:        cacheTTL,
		ids:             ids,
		severity:        policy.NewSeverityPolicy(),
		recommendations: policy.NewRecommendationPolicy(),
		now:             time.Now,
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

	relevant := policy.FilterEvidence(evidence, policy.RelevantEvidenceSpecification(request), maxLLMEvidenceItems)

	result, err := useCase.generator.Generate(ctx, request, relevant)
	if err != nil {
		return domain.AnalysisResult{}, fmt.Errorf("generate analysis: %w", err)
	}

	id, err := useCase.ids.NextID(ctx)
	if err != nil {
		return domain.AnalysisResult{}, fmt.Errorf("create analysis id: %w", err)
	}

	result.ID = id
	result.Evidence = evidence
	result.Severity = useCase.severity.Reconcile(result.Severity, policy.SeverityInput{
		Request:  request,
		Evidence: evidence,
	})
	result.RecommendedActions = useCase.recommendations.Apply(policy.RecommendationInput{
		Request:  request,
		Evidence: evidence,
		Result:   result,
	})
	result.CreatedAt = useCase.now().UTC()

	if err := useCase.repository.Save(ctx, result); err != nil {
		return domain.AnalysisResult{}, fmt.Errorf("save analysis: %w", err)
	}

	_ = useCase.cache.Save(ctx, domain.NewAnalysisContext(result), useCase.cacheTTL)

	return result, nil
}

// Get returns a previously stored analysis result.
func (useCase *Analysis) Get(ctx context.Context, analysisID string) (domain.AnalysisResult, error) {
	analysisID = strings.TrimSpace(analysisID)
	if analysisID == "" {
		return domain.AnalysisResult{}, fmt.Errorf("%w: analysis id is required", domain.ErrAnalysisNotFound)
	}

	result, err := useCase.repository.Find(ctx, analysisID)
	if err != nil {
		return domain.AnalysisResult{}, fmt.Errorf("find analysis: %w", err)
	}

	return result, nil
}

// List returns stored analyses using bounded pagination and provider-agnostic filters.
func (useCase *Analysis) List(ctx context.Context, filter domain.AnalysisListFilter) (domain.AnalysisList, error) {
	filter = normalizeAnalysisListFilter(filter)

	result, err := useCase.repository.ListAnalyses(ctx, filter)
	if err != nil {
		return domain.AnalysisList{}, fmt.Errorf("list analyses: %w", err)
	}

	return result, nil
}

func normalizeAnalysisListFilter(filter domain.AnalysisListFilter) domain.AnalysisListFilter {
	filter.Service = strings.TrimSpace(filter.Service)
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	if filter.Limit <= 0 {
		filter.Limit = defaultAnalysisListLimit
	}
	if filter.Limit > maxAnalysisListLimit {
		filter.Limit = maxAnalysisListLimit
	}

	return filter
}
