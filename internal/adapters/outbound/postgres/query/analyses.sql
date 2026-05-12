-- name: SaveAnalysis :exec
INSERT INTO analyses (
    id,
    summary,
    severity,
    confidence,
    affected_services,
    evidence,
    detected_anomalies,
    possible_root_causes,
    recommended_actions,
    code_level_insights,
    missing_evidence,
    created_at,
    updated_at
) VALUES (
    sqlc.arg(id),
    sqlc.arg(summary),
    sqlc.arg(severity),
    sqlc.arg(confidence),
    sqlc.arg(affected_services),
    sqlc.arg(evidence)::jsonb,
    sqlc.arg(detected_anomalies),
    sqlc.arg(possible_root_causes)::jsonb,
    sqlc.arg(recommended_actions)::jsonb,
    sqlc.arg(code_level_insights),
    sqlc.arg(missing_evidence),
    sqlc.arg(created_at),
    now()
)
ON CONFLICT (id) DO UPDATE SET
    summary = EXCLUDED.summary,
    severity = EXCLUDED.severity,
    confidence = EXCLUDED.confidence,
    affected_services = EXCLUDED.affected_services,
    evidence = EXCLUDED.evidence,
    detected_anomalies = EXCLUDED.detected_anomalies,
    possible_root_causes = EXCLUDED.possible_root_causes,
    recommended_actions = EXCLUDED.recommended_actions,
    code_level_insights = EXCLUDED.code_level_insights,
    missing_evidence = EXCLUDED.missing_evidence,
    created_at = EXCLUDED.created_at,
    updated_at = now();

-- name: FindAnalysis :one
SELECT
    id,
    summary,
    severity,
    confidence,
    affected_services,
    evidence,
    detected_anomalies,
    possible_root_causes,
    recommended_actions,
    code_level_insights,
    missing_evidence,
    created_at
FROM analyses
WHERE id = $1;
