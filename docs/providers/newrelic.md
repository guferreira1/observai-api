# New Relic

Read-only observability adapter for New Relic. Implements
`SignalCollector` over the GraphQL `actor.account.nrql` field.

## Authentication

Authentication uses the `API-Key: <user-api-key>` header. Generate a
**User API key** with the `Read NRQL queries` capability. The account id
must also be supplied.

## YAML / env wiring (legacy)

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

EU accounts use `https://api.eu.newrelic.com`.

## Admin DB configuration (W4+)

```json
{
  "type": "newrelic",
  "name": "nr-prod",
  "url": "https://api.newrelic.com",
  "timeoutMs": 15000,
  "signals": ["metrics"],
  "options": {"account_id": "1234567"},
  "credentials": "NRAK-...."
}
```

`credentials` carries the API key (encrypted at rest); `options.account_id`
is plaintext because it is not a secret.

## NRQL templates

Default templates (overridable):

- `transaction_duration_p95`: `SELECT percentile(duration, 95) FROM Transaction WHERE service.name = '{service}' SINCE 15 minutes ago` (unit `ms`)
- `error_rate`: `SELECT count(*) FROM TransactionError WHERE service.name = '{service}' SINCE 15 minutes ago` (unit `errors`)

`{service}` substitution escapes `'` and `\` so the embedded value cannot
break out of the NRQL string literal.

## Test connection

`POST /v1/admin/providers/{id}/test` issues `POST /graphql` with
`{ actor { user { email } } }`. `401`/`403` map to `unauthorized`;
GraphQL `errors[]` propagate as a sanitized error message.
