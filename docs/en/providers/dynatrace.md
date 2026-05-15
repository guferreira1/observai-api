# Dynatrace

Read-only observability adapter for Dynatrace.

## Authentication

Uses `Authorization: Api-Token <token>`.

Generate a token with at least:

- `metrics.read`
- `entities.read`

## Configuration

```yaml
observability:
  providers:
    - type: dynatrace
      name: dt-prod
      url: https://abc12345.live.dynatrace.com
      timeout: 15s
      signals: [metrics]
      options:
        api_token_ref: env:OBSERVAI_DT_TOKEN
```

## Templates

- `service_response_time_avg`: `builtin:service.response.time:filter(eq("dt.entity.service.name","{service}")):avg` (ms)
- `service_error_rate`: `builtin:service.errors.total.rate:filter(eq("dt.entity.service.name","{service}"))`

`{service}` values are escaped before substitution.

## Test endpoint

`POST /v1/admin/providers/{id}/test` validates with `GET /api/v2/clusterversion`.

