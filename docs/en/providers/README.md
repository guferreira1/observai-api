# Provider configuration

ObservAI uses the same provider abstraction for all observability and LLM integrations.

Provider and LLM declarations should be managed through the admin interface or
admin API after the API starts.

The admin plane persists provider definitions in the database, encrypts
credentials, supports test-connection before activation and hot-reloads active
adapters without restarting the API.

Use the admin API when scripting setup:

```http
POST /v1/admin/providers
POST /v1/admin/llm-providers
POST /v1/admin/providers/{id}/test
POST /v1/admin/llm-providers/{id}/test
POST /v1/admin/providers/{id}/activate
POST /v1/admin/llm-providers/{id}/activate
```

For self-hosted OpenAI-compatible gateways that do not require a token, use
`type: openai-compatible` with `options.auth: optional`. Hosted providers keep
requiring an API key by default.

```json
{
  "type": "openai-compatible",
  "name": "local-openai",
  "baseUrl": "http://llm-gateway:8080/v1",
  "model": "qwen2.5-coder",
  "options": {
    "auth": "optional"
  },
  "isActive": true
}
```

## Operational notes

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
