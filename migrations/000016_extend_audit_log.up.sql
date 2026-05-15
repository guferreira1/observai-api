ALTER TABLE audit_log
    ADD COLUMN action TEXT NOT NULL DEFAULT '',
    ADD COLUMN resource_type TEXT NOT NULL DEFAULT '',
    ADD COLUMN resource_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN metadata JSONB NOT NULL DEFAULT '{}'::JSONB;

CREATE INDEX audit_log_action_idx ON audit_log (action, created_at DESC) WHERE action <> '';
CREATE INDEX audit_log_resource_idx ON audit_log (resource_type, resource_id, created_at DESC) WHERE resource_type <> '';
CREATE INDEX audit_log_actor_idx ON audit_log (actor, created_at DESC) WHERE actor <> '';
