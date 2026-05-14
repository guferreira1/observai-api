ALTER TABLE analyses
    ADD CONSTRAINT analyses_severity_check CHECK (severity IN ('info', 'low', 'medium', 'high', 'critical')),
    ADD CONSTRAINT analyses_confidence_check CHECK (confidence IN ('low', 'medium', 'high'));

CREATE INDEX IF NOT EXISTS analyses_severity_created_at_idx ON analyses (severity, created_at DESC);
