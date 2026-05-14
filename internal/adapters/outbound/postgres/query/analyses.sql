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
    trace_id,
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
    sqlc.arg(trace_id),
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
    trace_id = EXCLUDED.trace_id,
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
    trace_id,
    created_at
FROM analyses
WHERE id = $1;

-- name: ListAnalyses :many
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
    trace_id,
    created_at
FROM analyses
WHERE (sqlc.narg(severity)::text IS NULL OR severity = sqlc.narg(severity)::text)
  AND (sqlc.narg(service)::text IS NULL OR sqlc.narg(service)::text = ANY(affected_services))
  AND (sqlc.narg(from_at)::timestamptz IS NULL OR created_at >= sqlc.narg(from_at)::timestamptz)
  AND (sqlc.narg(to_at)::timestamptz IS NULL OR created_at <= sqlc.narg(to_at)::timestamptz)
  AND (
        sqlc.narg(q)::text IS NULL
        OR summary ILIKE '%' || sqlc.narg(q)::text || '%'
        OR id ILIKE '%' || sqlc.narg(q)::text || '%'
      )
  AND (
        sqlc.narg(signal)::text IS NULL
        OR evidence @> jsonb_build_array(jsonb_build_object('Signal', sqlc.narg(signal)::text))
      )
  AND (
        sqlc.narg(provider)::text IS NULL
        OR evidence @> jsonb_build_array(jsonb_build_object('Provider', sqlc.narg(provider)::text))
      )
ORDER BY
    CASE WHEN sqlc.arg(sort_by)::text = 'severity' AND sqlc.arg(order_asc)::bool THEN
        CASE severity
            WHEN 'info' THEN 0
            WHEN 'low' THEN 1
            WHEN 'medium' THEN 2
            WHEN 'high' THEN 3
            WHEN 'critical' THEN 4
            ELSE 0
        END
    END ASC NULLS LAST,
    CASE WHEN sqlc.arg(sort_by)::text = 'severity' AND NOT sqlc.arg(order_asc)::bool THEN
        CASE severity
            WHEN 'info' THEN 0
            WHEN 'low' THEN 1
            WHEN 'medium' THEN 2
            WHEN 'high' THEN 3
            WHEN 'critical' THEN 4
            ELSE 0
        END
    END DESC NULLS LAST,
    CASE WHEN sqlc.arg(sort_by)::text = 'confidence' AND sqlc.arg(order_asc)::bool THEN
        CASE confidence
            WHEN 'low' THEN 1
            WHEN 'medium' THEN 2
            WHEN 'high' THEN 3
            ELSE 0
        END
    END ASC NULLS LAST,
    CASE WHEN sqlc.arg(sort_by)::text = 'confidence' AND NOT sqlc.arg(order_asc)::bool THEN
        CASE confidence
            WHEN 'low' THEN 1
            WHEN 'medium' THEN 2
            WHEN 'high' THEN 3
            ELSE 0
        END
    END DESC NULLS LAST,
    CASE WHEN sqlc.arg(sort_by)::text = 'createdAt' AND sqlc.arg(order_asc)::bool THEN created_at END ASC NULLS LAST,
    CASE WHEN sqlc.arg(sort_by)::text = 'createdAt' AND NOT sqlc.arg(order_asc)::bool THEN created_at END DESC NULLS LAST,
    id ASC
LIMIT sqlc.arg(result_limit)
OFFSET sqlc.arg(result_offset);

-- name: CountAnalyses :one
SELECT count(*)::int
FROM analyses
WHERE (sqlc.narg(severity)::text IS NULL OR severity = sqlc.narg(severity)::text)
  AND (sqlc.narg(service)::text IS NULL OR sqlc.narg(service)::text = ANY(affected_services))
  AND (sqlc.narg(from_at)::timestamptz IS NULL OR created_at >= sqlc.narg(from_at)::timestamptz)
  AND (sqlc.narg(to_at)::timestamptz IS NULL OR created_at <= sqlc.narg(to_at)::timestamptz)
  AND (
        sqlc.narg(q)::text IS NULL
        OR summary ILIKE '%' || sqlc.narg(q)::text || '%'
        OR id ILIKE '%' || sqlc.narg(q)::text || '%'
      )
  AND (
        sqlc.narg(signal)::text IS NULL
        OR evidence @> jsonb_build_array(jsonb_build_object('Signal', sqlc.narg(signal)::text))
      )
  AND (
        sqlc.narg(provider)::text IS NULL
        OR evidence @> jsonb_build_array(jsonb_build_object('Provider', sqlc.narg(provider)::text))
      );

-- name: AnalysesSeverityHistogram :many
SELECT
    severity,
    count(*)::int AS total
FROM analyses
WHERE (sqlc.narg(severity)::text IS NULL OR severity = sqlc.narg(severity)::text)
  AND (sqlc.narg(service)::text IS NULL OR sqlc.narg(service)::text = ANY(affected_services))
  AND (sqlc.narg(from_at)::timestamptz IS NULL OR created_at >= sqlc.narg(from_at)::timestamptz)
  AND (sqlc.narg(to_at)::timestamptz IS NULL OR created_at <= sqlc.narg(to_at)::timestamptz)
GROUP BY severity;

-- name: AnalysesConfidenceHistogram :many
SELECT
    confidence,
    count(*)::int AS total
FROM analyses
WHERE (sqlc.narg(severity)::text IS NULL OR severity = sqlc.narg(severity)::text)
  AND (sqlc.narg(service)::text IS NULL OR sqlc.narg(service)::text = ANY(affected_services))
  AND (sqlc.narg(from_at)::timestamptz IS NULL OR created_at >= sqlc.narg(from_at)::timestamptz)
  AND (sqlc.narg(to_at)::timestamptz IS NULL OR created_at <= sqlc.narg(to_at)::timestamptz)
GROUP BY confidence;

-- name: AnalysesTopServices :many
SELECT
    service::text AS service,
    count(*)::int AS total
FROM analyses, unnest(affected_services) AS service
WHERE (sqlc.narg(severity)::text IS NULL OR severity = sqlc.narg(severity)::text)
  AND (sqlc.narg(service_filter)::text IS NULL OR sqlc.narg(service_filter)::text = ANY(affected_services))
  AND (sqlc.narg(from_at)::timestamptz IS NULL OR created_at >= sqlc.narg(from_at)::timestamptz)
  AND (sqlc.narg(to_at)::timestamptz IS NULL OR created_at <= sqlc.narg(to_at)::timestamptz)
  AND service <> ''
  AND (sqlc.narg(q)::text IS NULL OR service ILIKE '%' || sqlc.narg(q)::text || '%')
GROUP BY service
ORDER BY total DESC, service ASC
LIMIT sqlc.arg(result_limit);

-- name: AnalysesTrendBuckets :many
SELECT
    date_trunc('day', created_at)::timestamptz AS bucket_start,
    count(*)::int AS total
FROM analyses
WHERE (sqlc.narg(severity)::text IS NULL OR severity = sqlc.narg(severity)::text)
  AND (sqlc.narg(service)::text IS NULL OR sqlc.narg(service)::text = ANY(affected_services))
  AND (sqlc.narg(from_at)::timestamptz IS NULL OR created_at >= sqlc.narg(from_at)::timestamptz)
  AND (sqlc.narg(to_at)::timestamptz IS NULL OR created_at <= sqlc.narg(to_at)::timestamptz)
GROUP BY bucket_start
ORDER BY bucket_start ASC;
