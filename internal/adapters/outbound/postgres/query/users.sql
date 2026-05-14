-- name: CreateUser :exec
INSERT INTO users (id, email, password_hash, role, is_active, created_at, updated_at)
VALUES (sqlc.arg(id), sqlc.arg(email), sqlc.arg(password_hash), sqlc.arg(role), sqlc.arg(is_active), sqlc.arg(created_at), sqlc.arg(updated_at));

-- name: FindUserByEmail :one
SELECT id, email, password_hash, role, is_active, created_at, updated_at, last_login_at
FROM users
WHERE email = sqlc.arg(email);

-- name: FindUserByID :one
SELECT id, email, password_hash, role, is_active, created_at, updated_at, last_login_at
FROM users
WHERE id = sqlc.arg(id);

-- name: ListUsers :many
SELECT id, email, password_hash, role, is_active, created_at, updated_at, last_login_at
FROM users
ORDER BY created_at DESC
LIMIT sqlc.arg(result_limit) OFFSET sqlc.arg(result_offset);

-- name: CountUsers :one
SELECT COUNT(*) FROM users;

-- name: UpdateUserProfile :exec
UPDATE users
SET email = sqlc.arg(email), updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: UpdateUserPassword :exec
UPDATE users
SET password_hash = sqlc.arg(password_hash), updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: UpdateUserRole :exec
UPDATE users
SET role = sqlc.arg(role), updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: SetUserActive :exec
UPDATE users
SET is_active = sqlc.arg(is_active), updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: TouchUserLastLogin :exec
UPDATE users
SET last_login_at = sqlc.arg(last_login_at), updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: DeleteUser :exec
DELETE FROM users
WHERE id = sqlc.arg(id);
