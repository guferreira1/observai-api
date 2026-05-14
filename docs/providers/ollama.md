# Ollama

Self-hosted LLM adapter targeting Ollama's `/api/chat` endpoint with
`format=json`.

## YAML

```yaml
llm:
  providers:
    - type: ollama
      name: local-ollama
      url: http://localhost:11434
      model: llama3
      timeout: 30s
  active: local-ollama
```

## Legacy environment variables

```env
OBSERVAI_OLLAMA_URL=http://localhost:11434
OBSERVAI_OLLAMA_MODEL=llama3
OBSERVAI_OLLAMA_TIMEOUT=30s
```

These continue to work and are auto-promoted to `llm.providers`.

## Authentication

Ollama servers are typically unauthenticated on a private network. When
authentication is needed, place a reverse proxy in front and use the
OpenAI-compatible adapter via that proxy.

## Health probe

`/api/tags` is consulted on `/readyz`.
