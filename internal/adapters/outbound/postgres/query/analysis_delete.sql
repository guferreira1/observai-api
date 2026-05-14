-- name: DeleteAnalysisByID :execrows
DELETE FROM analyses WHERE id = sqlc.arg(id);

-- name: DeleteAnalysesOlderThan :execrows
DELETE FROM analyses WHERE created_at < sqlc.arg(cutoff);
