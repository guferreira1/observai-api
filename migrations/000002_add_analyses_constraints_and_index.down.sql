DROP INDEX IF EXISTS analyses_severity_created_at_idx;

ALTER TABLE analyses
    DROP CONSTRAINT IF EXISTS analyses_severity_check,
    DROP CONSTRAINT IF EXISTS analyses_confidence_check;
