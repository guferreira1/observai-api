# Datadog

Read-only observability adapter for Datadog.  
Supports metrics collections through Datadog's v1 metrics query API.

## Authentication

Datadog requires both headers:

- `DD-API-KEY`: organization API key
- `DD-APPLICATION-KEY`: application key with `metrics_read` scope

The code stores both in a single credential string:

`<api_key>:<app_key>` in `options.credentials` when using adapter constructor.

## Configuration

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

Site-specific base URLs such as `api.datadoghq.eu` are supported.

## Templates

Common defaults:

- `service_request_latency_avg`: `avg:trace.servlet.request{service:{service}}` (ms)
- `service_error_rate`: `avg:trace.servlet.request.errors{service:{service}}` (rate)

`{service}` is sanitized before interpolation.

## Test endpoint

`POST /v1/admin/providers/{id}/test` validates connectivity.

## Admin DB payload

When using DB-backed provider config, the `credentials` field is encrypted at rest.

