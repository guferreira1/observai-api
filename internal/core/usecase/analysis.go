package usecase

import (
	"context"
	"errors"
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
	jobs            ports.AnalysisJobRepository
	enqueuer        ports.JobEnqueuer
	severity        policy.SeverityPolicy
	recommendations policy.RecommendationPolicy
	now             func() time.Time
}

// NewAnalysis creates an analysis use case.
//
// jobs and enqueuer are optional collaborators. When nil, the use case still
// exposes the synchronous Analyze entrypoint but SubmitAnalysis returns an
// error because async submission requires both dependencies.
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

// WithAsyncBackend attaches the job repository and enqueuer required for
// asynchronous analysis submission. Returns the same use case for chaining.
func (useCase *Analysis) WithAsyncBackend(jobs ports.AnalysisJobRepository, enqueuer ports.JobEnqueuer) *Analysis {
	useCase.jobs = jobs
	useCase.enqueuer = enqueuer
	return useCase
}

// Analyze executes a provider-agnostic observability analysis synchronously.
//
// It is preserved for in-process callers (the async worker, integration tests)
// that already hold an [domain.AnalysisRequest] and do not need the job lifecycle.
func (useCase *Analysis) Analyze(ctx context.Context, request domain.AnalysisRequest) (domain.AnalysisResult, error) {
	if err := request.Validate(); err != nil {
		return domain.AnalysisResult{}, err
	}
	id, err := useCase.ids.NextID(ctx)
	if err != nil {
		return domain.AnalysisResult{}, fmt.Errorf("create analysis id: %w", err)
	}
	return useCase.executeAnalyze(ctx, id, request)
}

// SubmitAnalysis validates the request, creates a pending job and enqueues it.
//
// Callers must poll GetJob to observe completion. The asynchronous backend
// (AnalysisJobRepository and JobEnqueuer) must have been attached via
// WithAsyncBackend before calling this method.
func (useCase *Analysis) SubmitAnalysis(ctx context.Context, request domain.AnalysisRequest) (domain.AnalysisJob, error) {
	if useCase.jobs == nil || useCase.enqueuer == nil {
		return domain.AnalysisJob{}, errors.New("analysis async backend not configured")
	}
	if err := request.Validate(); err != nil {
		return domain.AnalysisJob{}, err
	}

	jobID, err := useCase.ids.NextID(ctx)
	if err != nil {
		return domain.AnalysisJob{}, fmt.Errorf("create analysis job id: %w", err)
	}

	job := domain.AnalysisJob{
		ID:        jobID,
		Status:    domain.JobStatusPending,
		Request:   request,
		CreatedAt: useCase.now().UTC(),
	}
	if err := useCase.jobs.Create(ctx, job); err != nil {
		return domain.AnalysisJob{}, fmt.Errorf("create analysis job: %w", err)
	}

	if err := useCase.enqueuer.EnqueueAnalysis(ctx, job.ID); err != nil {
		return domain.AnalysisJob{}, fmt.Errorf("enqueue analysis job: %w", err)
	}

	return job, nil
}

// GetJob returns the current state of an asynchronous analysis job.
func (useCase *Analysis) GetJob(ctx context.Context, jobID string) (domain.AnalysisJob, error) {
	if useCase.jobs == nil {
		return domain.AnalysisJob{}, errors.New("analysis async backend not configured")
	}
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return domain.AnalysisJob{}, fmt.Errorf("%w: job id is required", domain.ErrJobNotFound)
	}

	job, err := useCase.jobs.Find(ctx, jobID)
	if err != nil {
		return domain.AnalysisJob{}, err
	}
	return job, nil
}

// RunAnalysisJob loads the pending job, executes the analysis and records the outcome.
//
// Workers (asynq handler, in-memory worker) invoke this method to perform the
// real work after a producer enqueued the job identifier.
func (useCase *Analysis) RunAnalysisJob(ctx context.Context, jobID string) error {
	if useCase.jobs == nil {
		return errors.New("analysis async backend not configured")
	}

	job, err := useCase.jobs.Find(ctx, jobID)
	if err != nil {
		return fmt.Errorf("load analysis job: %w", err)
	}
	if job.Status == domain.JobStatusCompleted {
		return nil
	}

	if err := useCase.jobs.MarkRunning(ctx, jobID, useCase.now().UTC()); err != nil {
		return fmt.Errorf("mark analysis job running: %w", err)
	}

	result, runErr := useCase.executeAnalyze(ctx, jobID, job.Request)
	if runErr != nil {
		failedAt := useCase.now().UTC()
		if markErr := useCase.jobs.MarkFailed(ctx, jobID, runErr.Error(), failedAt); markErr != nil {
			return fmt.Errorf("mark analysis job failed after %v: %w", runErr, markErr)
		}
		return runErr
	}

	if err := useCase.jobs.MarkCompleted(ctx, jobID, result.ID, useCase.now().UTC()); err != nil {
		return fmt.Errorf("mark analysis job completed: %w", err)
	}
	return nil
}

func (useCase *Analysis) executeAnalyze(ctx context.Context, analysisID string, request domain.AnalysisRequest) (domain.AnalysisResult, error) {
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

	result.ID = analysisID
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
