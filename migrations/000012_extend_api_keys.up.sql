ALTER TABLE api_keys
    ADD COLUMN description TEXT,
    ADD COLUMN expires_at TIMESTAMPTZ,
    ADD COLUMN scopes TEXT[] NOT NULL DEFAULT '{}';

UPDATE api_keys
SET scopes = ARRAY['admin:read', 'admin:write', 'analysis:read', 'analysis:write', 'chat:write']
WHERE scope = 'admin' AND cardinality(scopes) = 0;

UPDATE api_keys
SET scopes = ARRAY['analysis:read', 'analysis:write', 'chat:write']
WHERE scope = 'default' AND cardinality(scopes) = 0;

CREATE INDEX api_keys_expires_idx ON api_keys (expires_at) WHERE expires_at IS NOT NULL;
