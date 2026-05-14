# Dynatrace

Read-only observability adapter for Dynatrace. Implements `SignalCollector`
over the Metrics v2 API.

## Authentication

Authentication uses the `Authorization: Api-Token <token>` header. Generate
a token in **Dynatrace UI → Access tokens** with at least the following
scopes:

- `metrics.read`
- `entities.read`

Read-only scopes are enough; the adapter never mutates anything.

## YAML / env wiring (legacy)

```yaml
observability:
  providers:
    - type: dynatrace
      name: dt-prod
      url: https://abc12345.live.dynatrace.com
      timeout: 15s
      signals: [metrics]
      options:
        api_token: dt0c01....   # plaintext (development only)
        api_token_ref: env:OBSERVAI_DT_TOKEN  # safer
```

The `api_token_ref` form resolves the credential through the
`internal/adapters/outbound/credentials` dispatcher (`env:` or `file:`
schemes today).

## Admin DB configuration (W4+)

Send a `POST /v1/admin/providers` with:

```json
{
  "type": "dynatrace",
  "name": "dt-prod",
  "url": "https://abc12345.live.dynatrace.com",
  "timeoutMs": 15000,
  "signals": ["metrics"],
  "options": {"api_token_ref": "env:OBSERVAI_DT_TOKEN"},
  "credentials": "dt0c01...."
}
```

The `credentials` payload is encrypted at rest (AES-256-GCM) and only
decrypted when the adapter is instantiated. `GET` responses replace it
with a masked preview.

## Metric templates

Built-in default templates (override via the use case `Templates`
parameter when needed):

- `service_response_time_avg`: `builtin:service.response.time:filter(eq("dt.entity.service.name","{service}")):avg` (unit `ms`)
- `service_error_rate`: `builtin:service.errors.total.rate:filter(eq("dt.entity.service.name","{service}"))` (unit `rate`)

`{service}` is substituted with the affected service name from the
analysis request after escaping the `"` and `\` characters.

## Test connection

`POST /v1/admin/providers/{id}/test` issues `GET /api/v2/clusterversion`
with the supplied token. The response includes `reached`, `latencyMs`
and a sanitized `error` message.
