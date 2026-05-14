package usecase

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
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
	redaction       policy.ChainRedactor
	notifier        AnalysisCompletionNotifier
	now             func() time.Time

	activeCancelsMu sync.Mutex
	activeCancels   map[string]context.CancelFunc
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
		redaction:       policy.NewDefaultRedactor(),
		now:             time.Now,
		activeCancels:   make(map[string]context.CancelFunc),
	}
}

// WithAsyncBackend attaches the job repository and enqueuer required for
// asynchronous analysis submission. Returns the same use case for chaining.
func (useCase *Analysis) WithAsyncBackend(jobs ports.AnalysisJobRepository, enqueuer ports.JobEnqueuer) *Analysis {
	useCase.jobs = jobs
	useCase.enqueuer = enqueuer
	return useCase
}

// WithRedaction replaces the default redaction chain. Useful for tests that
// need to assert exact LLM input or for environments that have already
// scrubbed the source data upstream.
func (useCase *Analysis) WithRedaction(redaction policy.ChainRedactor) *Analysis {
	useCase.redaction = redaction
	return useCase
}

// AnalysisCompletionNotifier is invoked after a job transitions to completed
// or failed. Implementations dispatch the result to external listeners
// (webhooks, message buses) and must not block the caller; the worker
// loop ignores the returned error so a degraded notifier never poisons
// the analysis pipeline.
type AnalysisCompletionNotifier interface {
	NotifyAnalysisCompleted(ctx context.Context, result domain.AnalysisResult)
	NotifyAnalysisFailed(ctx context.Context, jobID string, request domain.AnalysisRequest, reason string)
}

// WithCompletionNotifier wires a notifier invoked after RunAnalysisJob
// transitions to a terminal state. Passing nil disables notifications.
func (useCase *Analysis) WithCompletionNotifier(notifier AnalysisCompletionNotifier) *Analysis {
	useCase.notifier = notifier
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
	return useCase.executeAnalyze(ctx, id, request, nil)
}

// phaseReporter receives transitions while the analysis executes so the worker
// can persist progress without leaking job-specific concerns into executeAnalyze.
type phaseReporter func(phase domain.JobPhase, progressPercent int)

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
		Phase:     domain.PhaseQueued,
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
// real work after a producer enqueued the job identifier. The worker context
// is wrapped so an inbound CancelJob call can abort the running analysis.
func (useCase *Analysis) RunAnalysisJob(ctx context.Context, jobID string) error {
	if useCase.jobs == nil {
		return errors.New("analysis async backend not configured")
	}

	job, err := useCase.jobs.Find(ctx, jobID)
	if err != nil {
		return fmt.Errorf("load analysis job: %w", err)
	}
	if job.Status == domain.JobStatusCompleted || job.Status == domain.JobStatusCanceled {
		return nil
	}

	runCtx, cancel := context.WithCancel(ctx)
	useCase.registerCancel(jobID, cancel)
	defer useCase.releaseCancel(jobID)

	if err := useCase.jobs.MarkRunning(runCtx, jobID, useCase.now().UTC()); err != nil {
		return fmt.Errorf("mark analysis job running: %w", err)
	}

	reporter := func(phase domain.JobPhase, progressPercent int) {
		_ = useCase.jobs.MarkPhase(runCtx, jobID, phase, progressPercent, useCase.now().UTC())
	}

	result, runErr := useCase.executeAnalyze(runCtx, jobID, job.Request, reporter)
	if runErr != nil {
		if errors.Is(runErr, context.Canceled) {
			finishedAt := useCase.now().UTC()
			_ = useCase.jobs.MarkCanceled(context.Background(), jobID, finishedAt)
			return nil
		}
		failedAt := useCase.now().UTC()
		if markErr := useCase.jobs.MarkFailed(context.Background(), jobID, runErr.Error(), failedAt); markErr != nil {
			return fmt.Errorf("mark analysis job failed after %v: %w", runErr, markErr)
		}
		if useCase.notifier != nil {
			useCase.notifier.NotifyAnalysisFailed(context.Background(), jobID, job.Request, runErr.Error())
		}
		return runErr
	}

	if err := useCase.jobs.MarkCompleted(runCtx, jobID, result.ID, useCase.now().UTC()); err != nil {
		return fmt.Errorf("mark analysis job completed: %w", err)
	}
	if useCase.notifier != nil {
		useCase.notifier.NotifyAnalysisCompleted(context.Background(), result)
	}
	return nil
}

// CancelJob persists a cancellation request for the supplied job.
//
// Completed, failed and already-canceled jobs are returned as-is so the call
// is idempotent. Pending jobs transition to canceled immediately; running
// jobs are marked canceled and any in-process worker on this instance has its
// context cancelled so it can abort cooperatively.
func (useCase *Analysis) CancelJob(ctx context.Context, jobID string) (domain.AnalysisJob, error) {
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

	if isTerminalJobStatus(job.Status) {
		return job, nil
	}

	finishedAt := useCase.now().UTC()
	if err := useCase.jobs.MarkCanceled(ctx, jobID, finishedAt); err != nil {
		return domain.AnalysisJob{}, fmt.Errorf("mark analysis job canceled: %w", err)
	}

	useCase.invokeCancel(jobID)

	canceled, findErr := useCase.jobs.Find(ctx, jobID)
	if findErr != nil {
		return domain.AnalysisJob{}, findErr
	}
	return canceled, nil
}

func isTerminalJobStatus(status domain.JobStatus) bool {
	switch status {
	case domain.JobStatusCompleted, domain.JobStatusFailed, domain.JobStatusCanceled:
		return true
	}
	return false
}

func (useCase *Analysis) registerCancel(jobID string, cancel context.CancelFunc) {
	useCase.activeCancelsMu.Lock()
	defer useCase.activeCancelsMu.Unlock()
	useCase.activeCancels[jobID] = cancel
}

func (useCase *Analysis) releaseCancel(jobID string) {
	useCase.activeCancelsMu.Lock()
	defer useCase.activeCancelsMu.Unlock()
	if cancel, ok := useCase.activeCancels[jobID]; ok {
		cancel()
		delete(useCase.activeCancels, jobID)
	}
}

func (useCase *Analysis) invokeCancel(jobID string) {
	useCase.activeCancelsMu.Lock()
	cancel, ok := useCase.activeCancels[jobID]
	useCase.activeCancelsMu.Unlock()
	if ok {
		cancel()
	}
}

func (useCase *Analysis) executeAnalyze(ctx context.Context, analysisID string, request domain.AnalysisRequest, reporter phaseReporter) (domain.AnalysisResult, error) {
	if err := request.Validate(); err != nil {
		return domain.AnalysisResult{}, err
	}

	report(reporter, domain.PhaseCollectingSignals, 10)
	evidence, err := useCase.collector.Collect(ctx, request)
	if err != nil {
		return domain.AnalysisResult{}, fmt.Errorf("collect analysis evidence: %w", err)
	}

	report(reporter, domain.PhaseNormalizing, 35)
	assignEvidenceIDs(evidence)
	relevant := policy.FilterEvidence(evidence, policy.RelevantEvidenceSpecification(request), maxLLMEvidenceItems)
	relevant = useCase.redaction.RedactEvidence(relevant)

	report(reporter, domain.PhaseCallingLLM, 55)
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

	report(reporter, domain.PhasePersisting, 85)
	if err := useCase.repository.Save(ctx, result); err != nil {
		return domain.AnalysisResult{}, fmt.Errorf("save analysis: %w", err)
	}

	_ = useCase.cache.Save(ctx, domain.NewAnalysisContext(result), useCase.cacheTTL)

	return result, nil
}

func report(reporter phaseReporter, phase domain.JobPhase, progressPercent int) {
	if reporter == nil {
		return
	}
	reporter(phase, progressPercent)
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

const (
	statsServiceTopN      = 10
	statsMaxAnalysisFetch = 1000
)

// Stats returns aggregated counts over stored analyses honoring the supplied filter.
//
// When the configured repository implements AnalysisStatsRepository the
// aggregation is delegated to SQL; otherwise the use case falls back to
// listing analyses and computing counts in Go (bounded by statsMaxAnalysisFetch).
func (useCase *Analysis) Stats(ctx context.Context, filter domain.AnalysisStatsFilter) (domain.AnalysisStats, error) {
	if stats, ok := useCase.repository.(ports.AnalysisStatsRepository); ok {
		return stats.AnalysisStats(ctx, filter, statsServiceTopN)
	}

	listFilter := domain.AnalysisListFilter{
		Limit:    statsMaxAnalysisFetch,
		Severity: filter.Severity,
		Service:  filter.Service,
		From:     filter.From,
		To:       filter.To,
	}

	page, err := useCase.repository.ListAnalyses(ctx, listFilter)
	if err != nil {
		return domain.AnalysisStats{}, fmt.Errorf("list analyses for stats: %w", err)
	}

	return computeAnalysisStats(page.Items, filter), nil
}

func computeAnalysisStats(items []domain.AnalysisResult, filter domain.AnalysisStatsFilter) domain.AnalysisStats {
	stats := domain.AnalysisStats{
		Total:        len(items),
		BySeverity:   map[domain.Severity]int{},
		ByConfidence: map[domain.Confidence]int{},
		From:         filter.From,
		To:           filter.To,
	}

	serviceCounts := map[string]int{}
	bucketCounts := map[time.Time]int{}
	for _, item := range items {
		if item.Severity != "" {
			stats.BySeverity[item.Severity]++
		}
		if item.Confidence != "" {
			stats.ByConfidence[item.Confidence]++
		}
		for _, service := range item.AffectedServices {
			if service == "" {
				continue
			}
			serviceCounts[service]++
		}
		bucket := item.CreatedAt.UTC().Truncate(24 * time.Hour)
		bucketCounts[bucket]++
	}

	stats.TopAffectedServices = topServiceCounts(serviceCounts, statsServiceTopN)
	stats.TrendBuckets = sortedTrendBuckets(bucketCounts)
	return stats
}

func topServiceCounts(counts map[string]int, top int) []domain.AnalysisStatsServiceCount {
	items := make([]domain.AnalysisStatsServiceCount, 0, len(counts))
	for service, count := range counts {
		items = append(items, domain.AnalysisStatsServiceCount{Service: service, Count: count})
	}
	sort.SliceStable(items, func(left, right int) bool {
		if items[left].Count == items[right].Count {
			return items[left].Service < items[right].Service
		}
		return items[left].Count > items[right].Count
	})
	if top > 0 && len(items) > top {
		items = items[:top]
	}
	return items
}

func sortedTrendBuckets(counts map[time.Time]int) []domain.AnalysisStatsTrendBucket {
	buckets := make([]domain.AnalysisStatsTrendBucket, 0, len(counts))
	for bucket, count := range counts {
		buckets = append(buckets, domain.AnalysisStatsTrendBucket{
			BucketStart: bucket,
			Count:       count,
		})
	}
	sort.SliceStable(buckets, func(left, right int) bool {
		return buckets[left].BucketStart.Before(buckets[right].BucketStart)
	})
	return buckets
}

// Services returns unique service names derived from stored analyses.
//
// When the configured repository implements AnalysisServiceCatalog the
// listing is delegated to SQL with the query and limit pushed down; otherwise
// the use case falls back to iterating over the latest analyses and
// deduplicating service names in memory.
func (useCase *Analysis) Services(ctx context.Context, query string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = statsServiceTopN
	}
	if catalog, ok := useCase.repository.(ports.AnalysisServiceCatalog); ok {
		return catalog.ListAffectedServices(ctx, strings.TrimSpace(query), limit)
	}

	page, err := useCase.repository.ListAnalyses(ctx, domain.AnalysisListFilter{Limit: statsMaxAnalysisFetch})
	if err != nil {
		return nil, fmt.Errorf("list analyses for services autocomplete: %w", err)
	}

	counts := map[string]int{}
	needle := strings.ToLower(strings.TrimSpace(query))
	for _, analysis := range page.Items {
		for _, service := range analysis.AffectedServices {
			service = strings.TrimSpace(service)
			if service == "" {
				continue
			}
			if needle != "" && !strings.Contains(strings.ToLower(service), needle) {
				continue
			}
			counts[service]++
		}
	}

	ranked := topServiceCounts(counts, limit)
	services := make([]string, 0, len(ranked))
	for _, item := range ranked {
		services = append(services, item.Service)
	}
	return services, nil
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

func assignEvidenceIDs(evidence []domain.Evidence) {
	for index := range evidence {
		if evidence[index].ID == "" {
			evidence[index].ID = "ev_" + strconv.Itoa(index+1)
		}
	}
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
