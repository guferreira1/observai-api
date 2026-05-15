# Anthropic

Adapter LLM para a API `/v1/messages` da Anthropic.

## Configuração

```yaml
llm:
  providers:
    - type: anthropic
      name: claude-sonnet
      base_url: https://api.anthropic.com/v1
      model: claude-sonnet-4-6
      api_key_env: env:OBSERVAI_ANTHROPIC_API_KEY
      timeout: 60s
  active: claude-sonnet
```

## Comportamento

- Autenticação via `x-api-key` (Bearer não é suportado).
- Health check executa uma chamada mínima de completion.
- O JSON é validado por parsing de resposta, pois a Anthropic não usa JSON mode como a OpenAI.
- Retry em `429` e `5xx` com backoff exponencial limitado.

## Health probe

Implementado com request de baixa carga de tokens para validação rápida.

