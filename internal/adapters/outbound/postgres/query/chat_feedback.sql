-- name: UpsertChatFeedback :exec
INSERT INTO analysis_chat_feedback (
    analysis_id,
    message_id,
    useful,
    reason,
    created_at,
    updated_at
) VALUES (
    sqlc.arg(analysis_id),
    sqlc.arg(message_id),
    sqlc.arg(useful),
    sqlc.arg(reason),
    sqlc.arg(created_at),
    sqlc.arg(created_at)
)
ON CONFLICT (analysis_id, message_id) DO UPDATE
SET
    useful = EXCLUDED.useful,
    reason = EXCLUDED.reason,
    updated_at = EXCLUDED.updated_at;
