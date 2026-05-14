-- name: CreateProviderConfig :exec
INSERT INTO provider_configurations (id, type, name, url, timeout_ms, signals, options, credentials_ciphertext, is_active, created_at, updated_at)
VALUES (sqlc.arg(id), sqlc.arg(type), sqlc.arg(name), sqlc.arg(url), sqlc.arg(timeout_ms), sqlc.arg(signals), sqlc.arg(options), sqlc.arg(credentials_ciphertext), sqlc.arg(is_active), sqlc.arg(created_at), sqlc.arg(updated_at));

-- name: FindProviderConfig :one
SELECT id, type, name, url, timeout_ms, signals, options, credentials_ciphertext, is_active, created_at, updated_at
FROM provider_configurations
WHERE id = sqlc.arg(id);

-- name: ListProviderConfigs :many
SELECT id, type, name, url, timeout_ms, signals, options, credentials_ciphertext, is_active, created_at, updated_at
FROM provider_configurations
ORDER BY created_at DESC
LIMIT sqlc.arg(result_limit) OFFSET sqlc.arg(result_offset);

-- name: ListActiveProviderConfigs :many
SELECT id, type, name, url, timeout_ms, signals, options, credentials_ciphertext, is_active, created_at, updated_at
FROM provider_configurations
WHERE is_active = true
ORDER BY name;

-- name: CountProviderConfigs :one
SELECT COUNT(*) FROM provider_configurations;

-- name: UpdateProviderConfig :exec
UPDATE provider_configurations
SET name = sqlc.arg(name),
    url = sqlc.arg(url),
    timeout_ms = sqlc.arg(timeout_ms),
    signals = sqlc.arg(signals),
    options = sqlc.arg(options),
    credentials_ciphertext = sqlc.arg(credentials_ciphertext),
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: SetProviderConfigActive :exec
UPDATE provider_configurations
SET is_active = sqlc.arg(is_active), updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: DeleteProviderConfig :exec
DELETE FROM provider_configurations
WHERE id = sqlc.arg(id);
