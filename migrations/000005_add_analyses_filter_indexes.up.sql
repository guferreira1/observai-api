CREATE INDEX IF NOT EXISTS analyses_created_at_idx ON analyses (created_at DESC);

CREATE INDEX IF NOT EXISTS analyses_severity_idx ON analyses (severity);

CREATE INDEX IF NOT EXISTS analyses_evidence_gin_idx ON analyses USING GIN (evidence jsonb_path_ops);
