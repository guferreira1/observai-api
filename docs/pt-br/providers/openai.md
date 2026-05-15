# OpenAI (compatível)

Adapter compatível com a API OpenAI `/v1/chat/completions`.  
Também cobre `azure`, `openrouter` e gateways compatíveis.

## Configuração

```yaml
llm:
  providers:
    - type: openai
      name: gpt-4o
      base_url: https://api.openai.com/v1
      model: gpt-4o-mini
      api_key_env: env:OBSERVAI_OPENAI_API_KEY
      timeout: 60s
  active: gpt-4o
```

## Observações

- `type` aceita aliases: `openai`, `azure`, `openrouter`.
- `base_url` é o endpoint do provedor. `url` é aceito por compatibilidade.
- `response_format: json_object` é usado quando prompts exigem saída estruturada.
- O timeout controla o orçamento da requisição.

## Health

Probe com `GET /models`.

