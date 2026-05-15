# New Relic

Read-only observability adapter using New Relic GraphQL endpoints.

## Authentication

- `API-Key: <user-api-key>` header.
- User API Key must include `Read NRQL queries`.
- Account ID is required in options.

## Configuration

```yaml
observability:
  providers:
    - type: newrelic
      name: nr-prod
      url: https://api.newrelic.com
      timeout: 15s
      signals: [metrics]
      options:
        api_key_ref: env:OBSERVAI_NEWRELIC_KEY
        account_id: "1234567"
```

EU endpoints:

`https://api.eu.newrelic.com`

## Templates

- `transaction_duration_p95`: `SELECT percentile(duration, 95) FROM Transaction WHERE service.name = '{service}' SINCE 15 minutes ago` (ms)
- `error_rate`: `SELECT count(*) FROM TransactionError WHERE service.name = '{service}' SINCE 15 minutes ago`

## Test endpoint

`POST /v1/admin/providers/{id}/test` validates GraphQL query execution.

