# Elasticsearch / OpenSearch

Read-only logs collector.

The same adapter supports both `elasticsearch` and `opensearch` types.

## Configuration

```yaml
observability:
  providers:
    - type: elasticsearch  # or opensearch
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
        # or
        api_key: env:OBSERVAI_ELASTIC_API_KEY
```

## Options

| Key | Default | Meaning |
| --- | --- | --- |
| `index` | `_all` | Target index or wildcard pattern. |
| `error_pattern` | `(?i)(error|exception|panic)` | Regex for suspicious log lines. |
| `service_field` | `service.name` | Field used to map service name. |
| `message_field` | `message` | Field containing log text. |
| `timestamp_field` | `@timestamp` | Time range field. |
| `username` | (empty) | Basic auth username. |
| `password` / `password_ref` | (empty) | Password or secret reference. |
| `api_key` | (empty) | API key authentication reference. |

`api_key` has precedence over basic auth.

