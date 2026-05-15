-- name: DeleteAnalysisByID :execrows
DELETE FROM analyses WHERE id = sqlc.arg(id);

-- name: DeleteAnalysesOlderThan :execrows
DELETE FROM analyses WHERE created_at < sqlc.arg(cutoff);

-- name: DeleteAnalysesKeepingNewest :execrows
DELETE FROM analyses
WHERE id IN (
    SELECT id FROM analyses
    ORDER BY created_at DESC
    OFFSET sqlc.arg(keep_count)
);
