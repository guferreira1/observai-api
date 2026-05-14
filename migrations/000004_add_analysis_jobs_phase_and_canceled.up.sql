ALTER TABLE analysis_jobs
    ADD COLUMN phase TEXT NOT NULL DEFAULT 'queued',
    ADD COLUMN progress_percent INT NOT NULL DEFAULT 0,
    ADD COLUMN phase_started_at TIMESTAMPTZ;

ALTER TABLE analysis_jobs
    DROP CONSTRAINT analysis_jobs_status_check,
    ADD CONSTRAINT analysis_jobs_status_check
        CHECK (status IN ('pending', 'running', 'completed', 'failed', 'canceled'));

ALTER TABLE analysis_jobs
    ADD CONSTRAINT analysis_jobs_phase_check
        CHECK (phase IN ('queued', 'collecting_signals', 'normalizing', 'calling_llm', 'persisting', 'done'));
