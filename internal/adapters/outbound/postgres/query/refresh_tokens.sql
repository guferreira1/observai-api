-- name: CreateRefreshToken :exec
INSERT INTO refresh_tokens (id, user_id, token_hash, family_id, expires_at, created_at)
VALUES (sqlc.arg(id), sqlc.arg(user_id), sqlc.arg(token_hash), sqlc.arg(family_id), sqlc.arg(expires_at), sqlc.arg(created_at));

-- name: FindRefreshTokenByHash :one
SELECT id, user_id, token_hash, family_id, expires_at, revoked_at, replaced_by, created_at
FROM refresh_tokens
WHERE token_hash = sqlc.arg(token_hash);

-- name: RevokeRefreshToken :exec
UPDATE refresh_tokens
SET revoked_at = sqlc.arg(revoked_at), replaced_by = sqlc.arg(replaced_by)
WHERE id = sqlc.arg(id) AND revoked_at IS NULL;

-- name: RevokeRefreshTokenFamily :exec
UPDATE refresh_tokens
SET revoked_at = sqlc.arg(revoked_at)
WHERE family_id = sqlc.arg(family_id) AND revoked_at IS NULL;

-- name: RevokeRefreshTokensForUser :exec
UPDATE refresh_tokens
SET revoked_at = sqlc.arg(revoked_at)
WHERE user_id = sqlc.arg(user_id) AND revoked_at IS NULL;

-- name: DeleteExpiredRefreshTokens :execrows
DELETE FROM refresh_tokens
WHERE expires_at < sqlc.arg(cutoff);
