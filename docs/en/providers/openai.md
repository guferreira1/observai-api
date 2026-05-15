# OpenAI-compatible

Adapter compatible with OpenAI-style `/v1/chat/completions`.  
Also covers `azure`, `openrouter`, and compatible gateways.

## Configuration

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

## Notes

- `type` accepts aliases: `openai`, `openai-compatible`, `azure`, `openrouter`.
- `base_url` is the provider endpoint. `url` is accepted for compatibility.
- `api_key_env` is required by default.
- Self-hosted gateways without token auth can use `options.auth: optional`.
- `response_format: json_object` is used when prompts require structured output.
- Timeout controls request-level call budget.

## Self-hosted gateway without token auth

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

Probed with `GET /models`.
