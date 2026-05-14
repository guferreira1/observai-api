# Datadog

Read-only observability adapter for Datadog. Implements `SignalCollector`
over the v1 metrics query API.

## Authentication

Datadog requires two headers for most read endpoints:

- `DD-API-KEY`: organization API key
- `DD-APPLICATION-KEY`: application key with at least `metrics_read`
  scope

Both are supplied to the adapter as a single colon-separated string
(`<api_key>:<app_key>`). The internal split happens in `client.go`.

## YAML / env wiring (legacy)

```yaml
observability:
  providers:
    - type: datadog
      name: dd-prod
      url: https://api.datadoghq.com
      timeout: 15s
      signals: [metrics]
      options:
        credentials_ref: env:OBSERVAI_DATADOG_KEYS  # "api_key:app_key"
```

Site-specific base URLs (`api.datadoghq.eu`, `api.us3.datadoghq.com`,
etc.) work transparently.

## Admin DB configuration (W4+)

```json
{
  "type": "datadog",
  "name": "dd-prod",
  "url": "https://api.datadoghq.com",
  "timeoutMs": 15000,
  "signals": ["metrics"],
  "credentials": "abcd1234:efgh5678"
}
```

`credentials` is encrypted at rest. The string keeps the
`api_key:app_key` format so the adapter does not need to read multiple
columns.

## Metric templates

Default templates (overridable):

- `service_request_latency_avg`: `avg:trace.servlet.request{service:{service}}` (unit `ms`)
- `service_error_rate`: `avg:trace.servlet.request.errors{service:{service}}` (unit `rate`)

`{service}` substitution strips `{}` to defend against tag-string
injection.

## Test connection

`POST /v1/admin/providers/{id}/test` issues `GET /api/v1/validate`.
`401`/`403` map to `unauthorized`; `5xx` returns `latencyMs` plus the
sanitized error.
