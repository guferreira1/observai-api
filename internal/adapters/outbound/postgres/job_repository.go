package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/guferreira1/observai-api/internal/adapters/outbound/postgres/sqlc"
	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/platform/observability"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AnalysisJobRepository stores asynchronous analysis jobs in PostgreSQL.
type AnalysisJobRepository struct {
	pool     *pgxpool.Pool
	queries  *sqlc.Queries
	observer observability.ProviderObserver
}

// NewAnalysisJobRepository creates a PostgreSQL analysis job repository.
//
// The provided pool must already be initialized. Sharing the pool with the
// existing AnalysisRepository keeps connection usage predictable.
func NewAnalysisJobRepository(pool *pgxpool.Pool, opts ...RepositoryOptions) *AnalysisJobRepository {
	observer := observability.ProviderObserver(observability.NoopProviderObserver{})
	if len(opts) > 0 && opts[0].Observer != nil {
		observer = opts[0].Observer
	}
	return &AnalysisJobRepository{
		pool:     pool,
		queries:  sqlc.New(pool),
		observer: observer,
	}
}

func (repository *AnalysisJobRepository) observe(operation string, startedAt time.Time, err error) {
	repository.observer.Observe("postgres", operation, time.Since(startedAt), err)
}

// Create stores a new analysis job.
func (repository *AnalysisJobRepository) Create(ctx context.Context, job domain.AnalysisJob) (err error) {
	startedAt := time.Now()
	defer func() { repository.observe("create_analysis_job", startedAt, err) }()

	requestPayload, err := json.Marshal(job.Request)
	if err != nil {
		return fmt.Errorf("marshal analysis job request: %w", err)
	}

	phase := job.Phase
	if phase == "" {
		phase = domain.PhaseQueued
	}

	params := sqlc.CreateAnalysisJobParams{
		ID:              job.ID,
		AnalysisID:      optionalText(job.AnalysisID),
		Status:          string(job.Status),
		Request:         requestPayload,
		ErrorMessage:    job.ErrorMessage,
		Attempt:         int32(job.Attempt),
		Phase:           string(phase),
		ProgressPercent: int32(job.ProgressPercent),
		PhaseStartedAt:  optionalTimestamp(job.PhaseStartedAt),
		CreatedAt:       pgtype.Timestamptz{Time: job.CreatedAt, Valid: !job.CreatedAt.IsZero()},
		StartedAt:       optionalTimestamp(job.StartedAt),
		FinishedAt:      optionalTimestamp(job.FinishedAt),
	}
	if err := repository.queries.CreateAnalysisJob(ctx, params); err != nil {
		return fmt.Errorf("insert analysis job: %w", err)
	}
	return nil
}

// Find returns an analysis job by identifier.
func (repository *AnalysisJobRepository) Find(ctx context.Context, jobID string) (job domain.AnalysisJob, err error) {
	startedAt := time.Now()
	defer func() { repository.observe("find_analysis_job", startedAt, err) }()

	row, err := repository.queries.FindAnalysisJob(ctx, jobID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.AnalysisJob{}, fmt.Errorf("%w: %s", domain.ErrJobNotFound, jobID)
	}
	if err != nil {
		return domain.AnalysisJob{}, fmt.Errorf("select analysis job: %w", err)
	}

	return toDomainAnalysisJob(row)
}

// MarkRunning transitions the job to running.
func (repository *AnalysisJobRepository) MarkRunning(ctx context.Context, jobID string, startedAt time.Time) (err error) {
	observedAt := time.Now()
	defer func() { repository.observe("mark_analysis_job_running", observedAt, err) }()

	return repository.queries.MarkAnalysisJobRunning(ctx, sqlc.MarkAnalysisJobRunningParams{
		ID:        jobID,
		StartedAt: pgtype.Timestamptz{Time: startedAt, Valid: !startedAt.IsZero()},
	})
}

// MarkCompleted transitions the job to completed and stores the analysis identifier.
func (repository *AnalysisJobRepository) MarkCompleted(ctx context.Context, jobID string, analysisID string, finishedAt time.Time) (err error) {
	observedAt := time.Now()
	defer func() { repository.observe("mark_analysis_job_completed", observedAt, err) }()

	return repository.queries.MarkAnalysisJobCompleted(ctx, sqlc.MarkAnalysisJobCompletedParams{
		ID:         jobID,
		AnalysisID: optionalText(analysisID),
		FinishedAt: pgtype.Timestamptz{Time: finishedAt, Valid: !finishedAt.IsZero()},
	})
}

// MarkFailed transitions the job to failed and stores the error message.
func (repository *AnalysisJobRepository) MarkFailed(ctx context.Context, jobID string, errorMessage string, finishedAt time.Time) (err error) {
	observedAt := time.Now()
	defer func() { repository.observe("mark_analysis_job_failed", observedAt, err) }()

	return repository.queries.MarkAnalysisJobFailed(ctx, sqlc.MarkAnalysisJobFailedParams{
		ID:           jobID,
		ErrorMessage: errorMessage,
		FinishedAt:   pgtype.Timestamptz{Time: finishedAt, Valid: !finishedAt.IsZero()},
	})
}

// MarkPhase records a progress transition for a running job.
func (repository *AnalysisJobRepository) MarkPhase(ctx context.Context, jobID string, phase domain.JobPhase, progressPercent int, at time.Time) (err error) {
	observedAt := time.Now()
	defer func() { repository.observe("mark_analysis_job_phase", observedAt, err) }()

	return repository.queries.MarkAnalysisJobPhase(ctx, sqlc.MarkAnalysisJobPhaseParams{
		ID:              jobID,
		Phase:           string(phase),
		ProgressPercent: int32(progressPercent),
		PhaseStartedAt:  pgtype.Timestamptz{Time: at, Valid: !at.IsZero()},
	})
}

// MarkCanceled transitions the job to canceled and records the finish timestamp.
func (repository *AnalysisJobRepository) MarkCanceled(ctx context.Context, jobID string, finishedAt time.Time) (err error) {
	observedAt := time.Now()
	defer func() { repository.observe("mark_analysis_job_canceled", observedAt, err) }()

	return repository.queries.MarkAnalysisJobCanceled(ctx, sqlc.MarkAnalysisJobCanceledParams{
		ID:         jobID,
		FinishedAt: pgtype.Timestamptz{Time: finishedAt, Valid: !finishedAt.IsZero()},
	})
}

func optionalTimestamp(value *time.Time) pgtype.Timestamptz {
	if value == nil || value.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *value, Valid: true}
}

func toDomainAnalysisJob(row sqlc.FindAnalysisJobRow) (domain.AnalysisJob, error) {
	var request domain.AnalysisRequest
	if err := json.Unmarshal(row.Request, &request); err != nil {
		return domain.AnalysisJob{}, fmt.Errorf("unmarshal analysis job request: %w", err)
	}

	job := domain.AnalysisJob{
		ID:              row.ID,
		Status:          domain.JobStatus(row.Status),
		Phase:           domain.JobPhase(row.Phase),
		ProgressPercent: int(row.ProgressPercent),
		Request:         request,
		ErrorMessage:    row.ErrorMessage,
		Attempt:         int(row.Attempt),
		CreatedAt:       row.CreatedAt.Time.UTC(),
	}
	if row.AnalysisID.Valid {
		job.AnalysisID = row.AnalysisID.String
	}
	if row.StartedAt.Valid {
		started := row.StartedAt.Time.UTC()
		job.StartedAt = &started
	}
	if row.PhaseStartedAt.Valid {
		phaseStartedAt := row.PhaseStartedAt.Time.UTC()
		job.PhaseStartedAt = &phaseStartedAt
	}
	if row.FinishedAt.Valid {
		finished := row.FinishedAt.Time.UTC()
		job.FinishedAt = &finished
	}

	return job, nil
}
