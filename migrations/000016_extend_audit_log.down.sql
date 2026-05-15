DROP INDEX IF EXISTS audit_log_actor_idx;
DROP INDEX IF EXISTS audit_log_resource_idx;
DROP INDEX IF EXISTS audit_log_action_idx;

ALTER TABLE audit_log
    DROP COLUMN IF EXISTS metadata,
    DROP COLUMN IF EXISTS resource_id,
    DROP COLUMN IF EXISTS resource_type,
    DROP COLUMN IF EXISTS action;
