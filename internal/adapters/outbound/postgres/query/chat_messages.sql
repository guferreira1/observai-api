-- name: CreateChatMessage :one
INSERT INTO analysis_chat_messages (
    analysis_id,
    role,
    content,
    evidence,
    created_at
) VALUES (
    sqlc.arg(analysis_id),
    sqlc.arg(role),
    sqlc.arg(content),
    sqlc.arg(evidence)::jsonb,
    now()
)
RETURNING id, analysis_id, role, content, evidence, created_at;

-- name: ListChatMessagesByAnalysis :many
SELECT
    id,
    analysis_id,
    role,
    content,
    evidence,
    created_at
FROM analysis_chat_messages
WHERE analysis_id = $1
ORDER BY created_at ASC, id ASC;
