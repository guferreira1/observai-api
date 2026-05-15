# Ollama

Self-hosted LLM adapter via `/api/chat` with JSON output.

## Configuration

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

## Legacy support

Legacy variables remain supported:

- `OBSERVAI_OLLAMA_URL`
- `OBSERVAI_OLLAMA_MODEL`
- `OBSERVAI_OLLAMA_TIMEOUT`

## Authentication

Usually no auth in private networks. Use reverse-proxy + API gateway when needed.

## Health

Probed with `/api/tags`.

