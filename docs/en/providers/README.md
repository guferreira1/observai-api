# Provider configuration

ObservAI uses the same provider abstraction for all observability and LLM integrations.

Provider declarations are grouped in two top-level blocks:

```yaml
observability:
  providers:
    - type: prometheus | loki | elasticsearch | opensearch | jaeger | tempo | otel
      name: observability-prod
      url: https://...
      timeout: 15s
      signals: [logs | metrics | traces | apm]
      options:
        key: value

llm:
  providers:
    - type: ollama | openai | azure | openrouter | anthropic
      name: llm-prod
      base_url: https://api.example.com/v1
      model: gpt-4o
      api_key_env: env:OBSERVAI_OPENAI_API_KEY
      timeout: 60s
  active: llm-prod
```

`active` selects the runtime LLM provider when more than one exists.

`options` are provider-specific key-values. Credentials must be provided through secure
references when possible:

- `env:VAR` (environment variable)
- `file:/absolute/path` (file secret)
- plain values for local dev.

## Operational notes

- Use the provider admin API for runtime changes without restart:
  `POST /v1/admin/providers` and `POST /v1/admin/llm-providers`.
- Validate connectivity before enabling a provider:
  `POST /v1/admin/providers/{id}/test`,
  `POST /v1/admin/llm-providers/{id}/test`.
- Activate/deactivate without deleting:
  `POST /v1/admin/providers/{id}/activate`,
  `/deactivate` and equivalent LLM endpoints.

## Supported providers

- [prometheus.md](./prometheus.md)
- [loki.md](./loki.md)
- [elasticsearch.md](./elasticsearch.md)
- [jaeger.md](./jaeger.md)
- [otel.md](./otel.md)
- [datadog.md](./datadog.md)
- [dynatrace.md](./dynatrace.md)
- [newrelic.md](./newrelic.md)
- [openai.md](./openai.md)
- [anthropic.md](./anthropic.md)
- [ollama.md](./ollama.md)

