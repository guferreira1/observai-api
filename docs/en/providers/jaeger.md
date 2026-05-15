# Jaeger

Read-only trace provider based on the Jaeger HTTP query API.

## Configuration

```yaml
observability:
  providers:
    - type: jaeger
      name: jaeger-prod
      url: http://jaeger-query.svc:16686
      timeout: 15s
      signals: [traces]
```

## Behavior

- `GET /api/traces/{traceID}` is used for trace enrichment.
- `traceID` comes from `AnalysisResult.TraceID` (analysis-driven) or `analysisID` fallback.
- Converts spans into normalized `domain.Span` data (duration in ms, parent/child links, status).

## Health

Readiness may call `/api/services`.

