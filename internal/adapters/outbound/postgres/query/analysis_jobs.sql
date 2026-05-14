-- name: CreateAnalysisJob :exec
INSERT INTO analysis_jobs (
    id,
    analysis_id,
    status,
    request,
    error_message,
    attempt,
    phase,
    progress_percent,
    phase_started_at,
    created_at,
    started_at,
    finished_at
) VALUES (
    sqlc.arg(id),
    sqlc.narg(analysis_id),
    sqlc.arg(status),
    sqlc.arg(request)::jsonb,
    sqlc.arg(error_message),
    sqlc.arg(attempt),
    sqlc.arg(phase),
    sqlc.arg(progress_percent),
    sqlc.narg(phase_started_at),
    sqlc.arg(created_at),
    sqlc.narg(started_at),
    sqlc.narg(finished_at)
);

-- name: FindAnalysisJob :one
SELECT
    id,
    analysis_id,
    status,
    request,
    error_message,
    attempt,
    phase,
    progress_percent,
    phase_started_at,
    created_at,
    started_at,
    finished_at
FROM analysis_jobs
WHERE id = $1;

-- name: MarkAnalysisJobRunning :exec
UPDATE analysis_jobs
SET
    status = 'running',
    attempt = attempt + 1,
    started_at = sqlc.arg(started_at),
    phase = 'collecting_signals',
    progress_percent = 0,
    phase_started_at = sqlc.arg(started_at)
WHERE id = sqlc.arg(id);

-- name: MarkAnalysisJobPhase :exec
UPDATE analysis_jobs
SET
    phase = sqlc.arg(phase),
    progress_percent = sqlc.arg(progress_percent),
    phase_started_at = sqlc.arg(phase_started_at)
WHERE id = sqlc.arg(id);

-- name: MarkAnalysisJobCompleted :exec
UPDATE analysis_jobs
SET
    status = 'completed',
    analysis_id = sqlc.arg(analysis_id),
    finished_at = sqlc.arg(finished_at),
    phase = 'done',
    progress_percent = 100
WHERE id = sqlc.arg(id);

-- name: MarkAnalysisJobFailed :exec
UPDATE analysis_jobs
SET
    status = 'failed',
    error_message = sqlc.arg(error_message),
    finished_at = sqlc.arg(finished_at)
WHERE id = sqlc.arg(id);

-- name: MarkAnalysisJobCanceled :exec
UPDATE analysis_jobs
SET
    status = 'canceled',
    finished_at = sqlc.arg(finished_at)
WHERE id = sqlc.arg(id);
