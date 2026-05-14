package ports

import (
	"context"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// AnalysisJobRepository stores and retrieves asynchronous analysis jobs.
type AnalysisJobRepository interface {
	Create(ctx context.Context, job domain.AnalysisJob) error
	Find(ctx context.Context, jobID string) (domain.AnalysisJob, error)
	MarkRunning(ctx context.Context, jobID string, startedAt time.Time) error
	MarkPhase(ctx context.Context, jobID string, phase domain.JobPhase, progressPercent int, at time.Time) error
	MarkCompleted(ctx context.Context, jobID string, analysisID string, finishedAt time.Time) error
	MarkFailed(ctx context.Context, jobID string, errorMessage string, finishedAt time.Time) error
	MarkCanceled(ctx context.Context, jobID string, finishedAt time.Time) error
}

// JobEnqueuer publishes analysis jobs to the background worker queue.
type JobEnqueuer interface {
	EnqueueAnalysis(ctx context.Context, jobID string) error
}
