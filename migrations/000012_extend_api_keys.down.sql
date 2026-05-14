DROP INDEX IF EXISTS api_keys_expires_idx;

ALTER TABLE api_keys
    DROP COLUMN IF EXISTS scopes,
    DROP COLUMN IF EXISTS expires_at,
    DROP COLUMN IF EXISTS description;
