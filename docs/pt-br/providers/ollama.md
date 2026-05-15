# Ollama

Adapter de LLM self-hosted via `/api/chat` com resposta JSON.

## Configuração

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

## Suporte legado

Variáveis antigas ainda funcionam:

- `OBSERVAI_OLLAMA_URL`
- `OBSERVAI_OLLAMA_MODEL`
- `OBSERVAI_OLLAMA_TIMEOUT`

## Autenticação

Normalmente sem auth em rede privada. Use reverse proxy/API gateway quando necessário.

## Health

Probe com `/api/tags`.

