-- name: AppendAuditEntry :exec
INSERT INTO audit_log (
    request_id,
    api_key_id,
    actor,
    method,
    path,
    status,
    duration_ms,
    remote,
    created_at
) VALUES (
    sqlc.arg(request_id),
    sqlc.arg(api_key_id),
    sqlc.arg(actor),
    sqlc.arg(method),
    sqlc.arg(path),
    sqlc.arg(status),
    sqlc.arg(duration_ms),
    sqlc.arg(remote),
    sqlc.arg(created_at)
);

-- name: ListAuditEntries :many
SELECT id, request_id, api_key_id, actor, method, path, status, duration_ms, remote, created_at
FROM audit_log
WHERE (sqlc.narg(api_key_id)::text IS NULL OR api_key_id = sqlc.narg(api_key_id)::text)
  AND (sqlc.narg(from_at)::timestamptz IS NULL OR created_at >= sqlc.narg(from_at)::timestamptz)
  AND (sqlc.narg(to_at)::timestamptz IS NULL OR created_at <= sqlc.narg(to_at)::timestamptz)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(result_limit) OFFSET sqlc.arg(result_offset);
