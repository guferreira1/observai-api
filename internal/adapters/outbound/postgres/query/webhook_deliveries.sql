-- name: CreateWebhookDelivery :exec
INSERT INTO webhook_deliveries (id, subscription_id, event, payload, status, attempt, next_attempt_at, created_at, updated_at)
VALUES (sqlc.arg(id), sqlc.arg(subscription_id), sqlc.arg(event), sqlc.arg(payload), sqlc.arg(status), sqlc.arg(attempt), sqlc.arg(next_attempt_at), sqlc.arg(created_at), sqlc.arg(updated_at));

-- name: FindWebhookDelivery :one
SELECT id, subscription_id, event, payload, status, attempt, last_error, response_status, next_attempt_at, delivered_at, created_at, updated_at
FROM webhook_deliveries
WHERE id = sqlc.arg(id);

-- name: ListWebhookDeliveries :many
SELECT id, subscription_id, event, payload, status, attempt, last_error, response_status, next_attempt_at, delivered_at, created_at, updated_at
FROM webhook_deliveries
ORDER BY created_at DESC
LIMIT sqlc.arg(result_limit) OFFSET sqlc.arg(result_offset);

-- name: ListPendingWebhookDeliveries :many
SELECT id, subscription_id, event, payload, status, attempt, last_error, response_status, next_attempt_at, delivered_at, created_at, updated_at
FROM webhook_deliveries
WHERE status = 'pending' AND (next_attempt_at IS NULL OR next_attempt_at <= sqlc.arg(cutoff))
ORDER BY created_at ASC
LIMIT sqlc.arg(result_limit);

-- name: MarkWebhookDeliveryDelivered :exec
UPDATE webhook_deliveries
SET status = 'delivered', response_status = sqlc.arg(response_status), delivered_at = sqlc.arg(delivered_at), updated_at = sqlc.arg(updated_at), last_error = NULL, next_attempt_at = NULL
WHERE id = sqlc.arg(id);

-- name: MarkWebhookDeliveryFailed :exec
UPDATE webhook_deliveries
SET status = 'failed', attempt = sqlc.arg(attempt), last_error = sqlc.arg(last_error), response_status = sqlc.arg(response_status), next_attempt_at = NULL, updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: ScheduleWebhookDeliveryRetry :exec
UPDATE webhook_deliveries
SET status = 'pending', attempt = sqlc.arg(attempt), last_error = sqlc.arg(last_error), response_status = sqlc.arg(response_status), next_attempt_at = sqlc.arg(next_attempt_at), updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);
