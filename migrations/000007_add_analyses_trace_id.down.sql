DROP INDEX IF EXISTS analyses_trace_id_idx;
ALTER TABLE analyses DROP COLUMN IF EXISTS trace_id;
