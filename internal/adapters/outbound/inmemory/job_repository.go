package inmemory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// AnalysisJobRepository stores analysis jobs in memory for local execution and tests.
type AnalysisJobRepository struct {
	mu   sync.RWMutex
	jobs map[string]domain.AnalysisJob
}

// NewAnalysisJobRepository creates an in-memory analysis job repository.
func NewAnalysisJobRepository() *AnalysisJobRepository {
	return &AnalysisJobRepository{jobs: make(map[string]domain.AnalysisJob)}
}

// Create stores a new analysis job.
func (repository *AnalysisJobRepository) Create(_ context.Context, job domain.AnalysisJob) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	repository.jobs[job.ID] = job
	return nil
}

// Find returns a job by identifier.
func (repository *AnalysisJobRepository) Find(_ context.Context, jobID string) (domain.AnalysisJob, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()

	job, ok := repository.jobs[jobID]
	if !ok {
		return domain.AnalysisJob{}, fmt.Errorf("%w: %s", domain.ErrJobNotFound, jobID)
	}
	return job, nil
}

// MarkRunning transitions the job to running and records the start timestamp.
func (repository *AnalysisJobRepository) MarkRunning(_ context.Context, jobID string, startedAt time.Time) error {
	return repository.transition(jobID, func(job *domain.AnalysisJob) {
		job.Status = domain.JobStatusRunning
		job.Attempt++
		startCopy := startedAt
		job.StartedAt = &startCopy
		job.Phase = domain.PhaseCollectingSignals
		job.PhaseStartedAt = &startCopy
	})
}

// MarkPhase records a progress transition for a running job.
func (repository *AnalysisJobRepository) MarkPhase(_ context.Context, jobID string, phase domain.JobPhase, progressPercent int, at time.Time) error {
	return repository.transition(jobID, func(job *domain.AnalysisJob) {
		job.Phase = phase
		job.ProgressPercent = progressPercent
		atCopy := at
		job.PhaseStartedAt = &atCopy
	})
}

// MarkCompleted transitions the job to completed and stores the analysis identifier.
func (repository *AnalysisJobRepository) MarkCompleted(_ context.Context, jobID string, analysisID string, finishedAt time.Time) error {
	return repository.transition(jobID, func(job *domain.AnalysisJob) {
		job.Status = domain.JobStatusCompleted
		job.Phase = domain.PhaseDone
		job.ProgressPercent = 100
		job.AnalysisID = analysisID
		finishCopy := finishedAt
		job.FinishedAt = &finishCopy
	})
}

// MarkFailed transitions the job to failed and stores the error message.
func (repository *AnalysisJobRepository) MarkFailed(_ context.Context, jobID string, errorMessage string, finishedAt time.Time) error {
	return repository.transition(jobID, func(job *domain.AnalysisJob) {
		job.Status = domain.JobStatusFailed
		job.ErrorMessage = errorMessage
		finishCopy := finishedAt
		job.FinishedAt = &finishCopy
	})
}

// MarkCanceled transitions the job to canceled and records the finish timestamp.
func (repository *AnalysisJobRepository) MarkCanceled(_ context.Context, jobID string, finishedAt time.Time) error {
	return repository.transition(jobID, func(job *domain.AnalysisJob) {
		job.Status = domain.JobStatusCanceled
		finishCopy := finishedAt
		job.FinishedAt = &finishCopy
	})
}

func (repository *AnalysisJobRepository) transition(jobID string, mutate func(*domain.AnalysisJob)) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	job, ok := repository.jobs[jobID]
	if !ok {
		return fmt.Errorf("%w: %s", domain.ErrJobNotFound, jobID)
	}
	mutate(&job)
	repository.jobs[jobID] = job
	return nil
}
