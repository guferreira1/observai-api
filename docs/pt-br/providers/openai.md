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

- `type` aceita aliases: `openai`, `openai-compatible`, `azure`, `openrouter`.
- `base_url` é o endpoint do provedor. `url` é aceito por compatibilidade.
- `api_key_env` é obrigatório por padrão.
- Gateways self-hosted sem autenticação por token podem usar `options.auth: optional`.
- `response_format: json_object` é usado quando prompts exigem saída estruturada.
- O timeout controla o orçamento da requisição.

## Gateway self-hosted sem token

```yaml
llm:
  providers:
    - type: openai-compatible
      name: local-openai
      base_url: http://llm-gateway:8080/v1
      model: qwen2.5-coder
      options:
        auth: optional
      timeout: 60s
  active: local-openai
```

## Health

Probe com `GET /models`.
