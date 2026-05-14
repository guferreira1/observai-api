# Elasticsearch / OpenSearch

Read-only logs collector using the `_search` aggregation API. The same
adapter handles both Elasticsearch and OpenSearch; declare the provider
type accordingly to make capabilities reporting accurate.

## YAML

```yaml
observability:
  providers:
    - type: elasticsearch         # or "opensearch"
      name: logs-prod
      url: https://logs.example.com
      timeout: 10s
      signals: [logs]
      options:
        index: app-logs-*
        error_pattern: "(?i).*(error|exception|panic).*"
        service_field: service.name
        message_field: message
        timestamp_field: "@timestamp"
        username: observai-read
        password_ref: env:OBSERVAI_ELASTIC_PASSWORD
        # OR
        api_key: env:OBSERVAI_ELASTIC_API_KEY
```

## Options

| Key               | Default            | Notes                                                  |
| ----------------- | ------------------ | ------------------------------------------------------ |
| `index`           | `_all`             | Index or index pattern to query.                       |
| `error_pattern`   | `(?i)(error\|exception\|panic)` | Regex applied via `regexp` query.        |
| `service_field`   | `service.name`     | Field used to match the affected service.              |
| `message_field`   | `message`          | Field carrying log text.                               |
| `timestamp_field` | `@timestamp`       | Field used for the time-range filter.                  |
| `username`        | empty              | Basic auth user. Pair with `password` or `password_ref`. |
| `password`        | empty              | Plaintext password (avoid).                            |
| `password_ref`    | empty              | CredentialStore reference (`env:`, `file:`).           |
| `api_key`         | empty              | CredentialStore reference for the `ApiKey` header.     |

API key auth takes precedence over basic auth when both are configured.
