# Loki

Read-only logs adapter using Loki `/loki/api/v1/query_range`.

## Configuration

```yaml
observability:
  providers:
    - type: loki
      name: loki-prod
      url: http://loki.svc:3100
      timeout: 10s
      signals: [logs]
```

## Behavior

- Query range is built per affected service.
- Default pattern counts events matching `(?i)(error|exception|panic)`.
- Step defaults to 60s and is bounded.

## Health

Readiness may call `/ready`.

