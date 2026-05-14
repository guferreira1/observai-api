# Jaeger

Read-only trace provider targeting Jaeger's HTTP query API.

## YAML

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

- `GET /api/traces/{traceID}` returns the trace. The trace ID is taken
  from `AnalysisResult.TraceID` (populated by the analysis use case) or
  the analysis ID as fallback.
- Spans are converted into `domain.Span` preserving service name (from
  the Jaeger process map), parent/child references, durations in
  milliseconds and a status derived from the `error` and
  `otel.status_code` tags.

## Health probe

`/api/services` is consulted on `/readyz`.
