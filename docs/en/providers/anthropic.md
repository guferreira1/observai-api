# Anthropic

LLM adapter for Anthropic's `/v1/messages` API.

## Configuration

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

## Behavior

- Authentication uses `x-api-key` (Bearer is not supported).
- Health check requests a tiny completion.
- JSON structure is enforced by response parsing because Anthropic responses are not
  OpenAI-compatible in JSON mode.
- Retries on `429` and `5xx` with bounded exponential backoff.

## Health probe

Configured as a lightweight request with a very small token budget.

