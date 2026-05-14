ALTER TABLE analyses ADD COLUMN trace_id TEXT NOT NULL DEFAULT '';

CREATE INDEX analyses_trace_id_idx ON analyses (trace_id)
WHERE trace_id <> '';
