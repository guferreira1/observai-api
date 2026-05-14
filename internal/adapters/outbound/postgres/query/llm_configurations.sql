-- name: CreateLLMConfig :exec
INSERT INTO llm_configurations (id, type, name, base_url, model, timeout_ms, api_key_ciphertext, options, is_active, created_at, updated_at)
VALUES (sqlc.arg(id), sqlc.arg(type), sqlc.arg(name), sqlc.arg(base_url), sqlc.arg(model), sqlc.arg(timeout_ms), sqlc.arg(api_key_ciphertext), sqlc.arg(options), sqlc.arg(is_active), sqlc.arg(created_at), sqlc.arg(updated_at));

-- name: FindLLMConfig :one
SELECT id, type, name, base_url, model, timeout_ms, api_key_ciphertext, options, is_active, created_at, updated_at
FROM llm_configurations
WHERE id = sqlc.arg(id);

-- name: ListLLMConfigs :many
SELECT id, type, name, base_url, model, timeout_ms, api_key_ciphertext, options, is_active, created_at, updated_at
FROM llm_configurations
ORDER BY created_at DESC
LIMIT sqlc.arg(result_limit) OFFSET sqlc.arg(result_offset);

-- name: FindActiveLLMConfig :one
SELECT id, type, name, base_url, model, timeout_ms, api_key_ciphertext, options, is_active, created_at, updated_at
FROM llm_configurations
WHERE is_active = true
LIMIT 1;

-- name: CountLLMConfigs :one
SELECT COUNT(*) FROM llm_configurations;

-- name: UpdateLLMConfig :exec
UPDATE llm_configurations
SET name = sqlc.arg(name),
    base_url = sqlc.arg(base_url),
    model = sqlc.arg(model),
    timeout_ms = sqlc.arg(timeout_ms),
    api_key_ciphertext = sqlc.arg(api_key_ciphertext),
    options = sqlc.arg(options),
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: DeactivateAllLLMConfigs :exec
UPDATE llm_configurations
SET is_active = false, updated_at = sqlc.arg(updated_at)
WHERE is_active = true;

-- name: ActivateLLMConfig :exec
UPDATE llm_configurations
SET is_active = true, updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: DeleteLLMConfig :exec
DELETE FROM llm_configurations
WHERE id = sqlc.arg(id);
