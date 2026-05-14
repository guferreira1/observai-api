-- name: CreateAPIKey :exec
INSERT INTO api_keys (id, name, key_hash, scope, created_at)
VALUES (sqlc.arg(id), sqlc.arg(name), sqlc.arg(key_hash), sqlc.arg(scope), sqlc.arg(created_at));

-- name: FindAPIKeyByHash :one
SELECT id, name, key_hash, scope, created_at, last_used_at, revoked_at
FROM api_keys
WHERE key_hash = sqlc.arg(key_hash)
  AND revoked_at IS NULL;

-- name: ListAPIKeys :many
SELECT id, name, key_hash, scope, created_at, last_used_at, revoked_at
FROM api_keys
ORDER BY created_at DESC
LIMIT sqlc.arg(result_limit) OFFSET sqlc.arg(result_offset);

-- name: RevokeAPIKey :exec
UPDATE api_keys
SET revoked_at = sqlc.arg(revoked_at)
WHERE id = sqlc.arg(id) AND revoked_at IS NULL;

-- name: TouchAPIKeyLastUsed :exec
UPDATE api_keys
SET last_used_at = sqlc.arg(last_used_at)
WHERE id = sqlc.arg(id);
