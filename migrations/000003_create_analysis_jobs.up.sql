CREATE TABLE analysis_jobs (
    id TEXT PRIMARY KEY,
    analysis_id TEXT,
    status TEXT NOT NULL,
    request JSONB NOT NULL,
    error_message TEXT NOT NULL DEFAULT '',
    attempt INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    CONSTRAINT analysis_jobs_status_check CHECK (status IN ('pending', 'running', 'completed', 'failed'))
);

CREATE INDEX analysis_jobs_status_idx ON analysis_jobs (status);
CREATE INDEX analysis_jobs_created_at_idx ON analysis_jobs (created_at DESC);
CREATE INDEX analysis_jobs_active_idx ON analysis_jobs (created_at) WHERE status IN ('pending', 'running');
