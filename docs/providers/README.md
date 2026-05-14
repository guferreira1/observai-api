# Provider configuration

ObservAI talks to observability and LLM providers through a single
dispatcher map keyed by provider type. Operators declare providers in
`config.yaml` (or via environment variables for legacy single-provider
mode) and the factory wires the appropriate adapter at startup.

Each provider page in this directory documents:

- the supported types and aliases
- mandatory and optional YAML fields
- environment variable equivalents
- a minimal working example

Two top-level blocks exist:

```yaml
observability:
  providers:
    - type: <prometheus | loki | elasticsearch | opensearch | jaeger | tempo | otel | otlp>
      name: <operator-facing identifier>
      url: <reachable URL>
      signals: [logs | metrics | traces | apm]
      timeout: 10s
      options: {}  # provider-specific

llm:
  providers:
    - type: <ollama | openai | azure | openrouter | anthropic>
      name: <operator-facing identifier>
      url: <reachable URL>
      base_url: <override; openai-compatible only>
      model: <model identifier>
      api_key_env: env:OBSERVAI_*_API_KEY  # ref scheme env: or file:
      timeout: 60s
  active: <name or type of the provider used at runtime>
```

The legacy single-provider env vars `OBSERVAI_PROMETHEUS_URL` and
`OBSERVAI_OLLAMA_URL` continue to work; they are automatically promoted
to entries in the lists above when present.

## Authentication references

Provider secrets (API keys, basic auth passwords) are never stored in
YAML directly. The fields use the `CredentialStore` reference format:

| Reference            | Resolution                                      |
| -------------------- | ----------------------------------------------- |
| `env:VAR_NAME`       | reads the named environment variable            |
| `file:/abs/path`     | reads the file contents, stripping whitespace   |
| `plain-text-value`   | accepted as-is (only for local/dev convenience) |

## Supported providers

| Page                                | Capability         |
| ----------------------------------- | ------------------ |
| [prometheus.md](./prometheus.md)    | metrics            |
| [loki.md](./loki.md)                | logs               |
| [elasticsearch.md](./elasticsearch.md) | logs            |
| [jaeger.md](./jaeger.md)            | traces             |
| [otel.md](./otel.md)                | traces (Tempo/OTLP)|
| [ollama.md](./ollama.md)            | LLM (self-hosted)  |
| [openai.md](./openai.md)            | LLM (OpenAI/Azure/OpenRouter) |
| [anthropic.md](./anthropic.md)      | LLM                |
