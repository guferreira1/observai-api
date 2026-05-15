# Configuração de provedores

O ObservAI utiliza a mesma abstração para todos os provedores de observabilidade e LLM.

As declarações ficam em dois blocos:

```yaml
observability:
  providers:
    - type: prometheus | loki | elasticsearch | opensearch | jaeger | tempo | otel
      name: observability-prod
      url: https://...
      timeout: 15s
      signals: [logs | metrics | traces | apm]
      options:
        chave: valor

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

`active` define qual provedor de LLM será usado em runtime quando houver mais de um.

`options` é um mapa específico de cada provedor. Sempre que possível, use referências
seguras para credenciais:

- `env:VAR` (variável de ambiente)
- `file:/caminho/absoluto` (arquivo secreto)
- valor literal para desenvolvimento local.

## Regras operacionais

- Para mudar configuração sem reinício, use API de admin:
  `POST /v1/admin/providers` e `POST /v1/admin/llm-providers`.
- Valide conectividade antes de ativar:
  `POST /v1/admin/providers/{id}/test`,
  `POST /v1/admin/llm-providers/{id}/test`.
- Ative/desative sem remover:
  `POST /v1/admin/providers/{id}/activate`,
  `/deactivate` e equivalentes de LLM.

## Provedores disponíveis

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

