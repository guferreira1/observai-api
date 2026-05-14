# OTel / Tempo

Read-only trace provider that consumes OTLP-JSON over HTTP. The same
adapter handles Grafana Tempo and any backend that exposes the standard
OpenTelemetry HTTP trace surface.

## YAML

Accepted type aliases: `otel`, `otlp`, `tempo`.

```yaml
observability:
  providers:
    - type: tempo
      name: tempo-prod
      url: http://tempo.svc:3200
      timeout: 15s
      signals: [traces]
```

## Behavior

- `GET /api/traces/{traceID}` returns the trace as OTLP-JSON
  (`resource_spans` or legacy `batches`).
- Spans are flattened across batches; service name is read from the
  `service.name` resource attribute; parent/child relations come from
  `parent_span_id`.
- Status is derived from the OTel `status.code` (`2` → error, `1` → ok).

## Health probe

`/api/echo` is consulted when available.
