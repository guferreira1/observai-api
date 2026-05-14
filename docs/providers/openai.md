# OpenAI-compatible

Single adapter that covers OpenAI, Azure OpenAI, OpenRouter and any
self-hosted gateway that mirrors the `/v1/chat/completions` schema.

Accepted type aliases: `openai`, `azure`, `openrouter`.

## YAML

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

## Fields

| Field          | Required | Notes                                                            |
| -------------- | -------- | ---------------------------------------------------------------- |
| `type`         | yes      | `openai`, `azure` or `openrouter`.                               |
| `base_url`     | no       | Override per provider. Default `https://api.openai.com/v1`.      |
| `url`          | no       | Legacy alias for `base_url`.                                     |
| `model`        | yes      | Model identifier accepted by the upstream API.                   |
| `api_key_env`  | yes      | CredentialStore reference. Bare names are treated as `env:NAME`. |
| `timeout`      | no       | Request timeout. Defaults to 60s.                                |

## JSON output

The adapter requests `response_format={"type":"json_object"}` whenever the
prompt expects JSON. Receivers must serve OpenAI's JSON-mode contract.

## Health probe

`GET /models` is consulted on `/readyz`.
