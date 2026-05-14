-- name: CreateWebhookSubscription :exec
INSERT INTO webhook_subscriptions (id, name, url, secret, event, created_at)
VALUES (sqlc.arg(id), sqlc.arg(name), sqlc.arg(url), sqlc.arg(secret), sqlc.arg(event), sqlc.arg(created_at));

-- name: ListWebhookSubscriptions :many
SELECT id, name, url, secret, event, created_at, disabled_at
FROM webhook_subscriptions
ORDER BY created_at DESC
LIMIT sqlc.arg(result_limit) OFFSET sqlc.arg(result_offset);

-- name: ListActiveWebhooksForEvent :many
SELECT id, name, url, secret, event, created_at, disabled_at
FROM webhook_subscriptions
WHERE event = sqlc.arg(event) AND disabled_at IS NULL;

-- name: DisableWebhookSubscription :exec
UPDATE webhook_subscriptions
SET disabled_at = sqlc.arg(disabled_at)
WHERE id = sqlc.arg(id) AND disabled_at IS NULL;
