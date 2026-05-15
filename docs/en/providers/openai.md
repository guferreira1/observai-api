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

- `type` accepts aliases: `openai`, `azure`, `openrouter`.
- `base_url` is the provider endpoint. `url` is accepted for compatibility.
- `response_format: json_object` is used when prompts require structured output.
- Timeout controls request-level call budget.

## Health

Probed with `GET /models`.

