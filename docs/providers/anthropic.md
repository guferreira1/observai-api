# Anthropic

LLM adapter targeting Anthropic's `/v1/messages` API.

## YAML

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

- Authentication uses the `x-api-key` header (Anthropic does not accept
  `Bearer`). The `anthropic-version` header is pinned to a known-good
  value (`2023-06-01`) to avoid silent upstream changes.
- JSON output is enforced through a strict system prompt suffix because
  `/v1/messages` does not implement OpenAI-style `response_format`.
- Retries 429 and 5xx with bounded exponential backoff.

## Health probe

The adapter issues a 1-token completion against the configured model.
Hardware-cheap but consumes a tiny amount of Anthropic quota.
