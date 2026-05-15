-- name: DeleteAnalysisByID :execrows
DELETE FROM analyses WHERE id = sqlc.arg(id);

-- name: CountAnalysesOlderThan :one
SELECT COUNT(*) FROM analyses WHERE created_at < sqlc.arg(cutoff);

-- name: DeleteAnalysesOlderThan :execrows
DELETE FROM analyses WHERE created_at < sqlc.arg(cutoff);

-- name: CountAnalysesExceedingNewest :one
SELECT COUNT(*) FROM (
    SELECT id FROM analyses
    ORDER BY created_at DESC
    OFFSET sqlc.arg(keep_count)
) retained_overflow;

-- name: DeleteAnalysesKeepingNewest :execrows
DELETE FROM analyses
WHERE id IN (
    SELECT id FROM analyses
    ORDER BY created_at DESC
    OFFSET sqlc.arg(keep_count)
);
