# OTel / Tempo

Read-only trace adapter for OTLP-compatible backends, including Tempo.

## Configuration

```yaml
observability:
  providers:
    - type: tempo
      name: tempo-prod
      url: http://tempo.svc:3200
      timeout: 15s
      signals: [traces]
```

Accepted aliases: `otel`, `otlp`, `tempo`.

## Behavior

- Reads `OTLP-JSON` traces from `/api/traces/{traceID}`.
- Flattens resource spans and extracts:
  - service name from `service.name`
  - parent-child relations from `parent_span_id`
  - status from OpenTelemetry status code.

## Health

Uses `/api/echo` when available.

