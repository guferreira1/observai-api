# Loki

Read-only logs collector using Loki's `query_range` HTTP API.

## YAML

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

- Hits `/loki/api/v1/query_range` for each affected service.
- Default template counts log events matching `(?i)(error|exception|panic)`
  inside the request time window per service.
- Step defaults to 60s and is bounded to keep aggregation cheap.

## Health probe

`/ready` is consulted on `/readyz`.
