ALTER TABLE analysis_jobs
    DROP CONSTRAINT IF EXISTS analysis_jobs_phase_check;

ALTER TABLE analysis_jobs
    DROP CONSTRAINT analysis_jobs_status_check,
    ADD CONSTRAINT analysis_jobs_status_check
        CHECK (status IN ('pending', 'running', 'completed', 'failed'));

ALTER TABLE analysis_jobs
    DROP COLUMN IF EXISTS phase_started_at,
    DROP COLUMN IF EXISTS progress_percent,
    DROP COLUMN IF EXISTS phase;
