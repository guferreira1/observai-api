# Prometheus

Read-only metrics collector.

## Configuration

```yaml
observability:
  providers:
    - type: prometheus
      name: prom-prod
      url: http://prometheus.svc:9090
      timeout: 10s
      signals: [metrics]
```

## Legacy compatibility

Legacy env variables still supported:

- `OBSERVAI_PROMETHEUS_URL`
- `OBSERVAI_PROMETHEUS_TIMEOUT`

## Behavior

- Executes instant queries against `/api/v1/query`.
- Templates are fixed for provider safety; raw user query is not forwarded.
- Default template checks `up` availability per affected service.
- Retries transient failures (network and `5xx`) with backoff+jitter.

## Health

Readiness may use `/-/healthy`.

