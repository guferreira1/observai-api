# Architecture rules

## Main style

ObservAI API follows hexagonal architecture.

The domain and use cases are the center of the system. Frameworks, databases, external APIs, queues, HTTP handlers, provider SDKs and LLM SDKs stay outside the core.

## Dependency direction

Allowed direction:

```txt
inbound adapters -> use cases -> ports/domain <- outbound adapters
```

The core defines ports. Adapters implement ports.

## Provider independence

Provider-specific behavior must stay in outbound adapters.

The same use case should work with different providers when the provider implements the required port.

Examples:

- Dynatrace, Datadog, New Relic and Elastic APM adapters implement APM ports.
- Elasticsearch, OpenSearch and Loki adapters implement log ports.
- Prometheus and compatible systems implement metric ports.
- Jaeger and OpenTelemetry-compatible systems implement trace ports.
- OpenAI, Anthropic, Gemini, OpenRouter, Ollama and local models implement LLM ports.

## Suggested package shape

```txt
cmd/
internal/core/domain/
internal/core/ports/
internal/core/usecase/
internal/adapters/inbound/
internal/adapters/outbound/
internal/platform/
```

## Boundaries

- Domain models must not depend on transport, persistence or provider models.
- Use cases orchestrate business flow and depend only on ports.
- Adapters convert external data into internal models.
- Provider SDK types must not leak into use cases.
- HTTP DTOs must not become domain entities.
