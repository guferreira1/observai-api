# Prometheus

Read-only metrics collector using the Prometheus instant query HTTP API.

## YAML

```yaml
observability:
  providers:
    - type: prometheus
      name: prom-prod
      url: http://prometheus.svc:9090
      timeout: 10s
      signals: [metrics]
```

## Legacy environment variables

The single-provider legacy fields continue to work:

```env
OBSERVAI_PROMETHEUS_URL=http://prometheus.svc:9090
OBSERVAI_PROMETHEUS_TIMEOUT=10s
```

When present they are auto-migrated into `observability.providers`.

## Behavior

- Hits `/api/v1/query`. Template-based queries only; raw user PromQL is
  never forwarded.
- Default template counts `up` per affected service.
- Retries transient failures (network errors, 5xx) with bounded
  exponential backoff and full jitter.

## Health probe

`/-/healthy` is consulted on `/readyz` to report the provider as ready.
