# Configuração de provedores

O ObservAI utiliza a mesma abstração para todos os provedores de observabilidade e LLM.

As declarações de provedores e LLMs devem ser gerenciadas pela interface
administrativa ou API admin depois que a API sobe.

O plano admin persiste as definições no banco, criptografa credenciais, permite
testar conexão antes da ativação e recarrega adaptadores ativos sem reiniciar a
API.

Use a API admin quando precisar automatizar setup:

```http
POST /v1/admin/providers
POST /v1/admin/llm-providers
POST /v1/admin/providers/{id}/test
POST /v1/admin/llm-providers/{id}/test
POST /v1/admin/providers/{id}/activate
POST /v1/admin/llm-providers/{id}/activate
```

Para gateways self-hosted compatíveis com OpenAI que não exigem token, use
`type: openai-compatible` com `options.auth: optional`. Provedores hospedados
continuam exigindo API key por padrão.

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

## Regras operacionais

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
