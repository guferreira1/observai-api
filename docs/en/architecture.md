# Architecture

ObservAI API is a hexagonal Go backend that adds an AI analysis layer above
observability systems and operational stores.

All provider-specific and framework concerns are implemented in adapters.  
The core domain only imports standard library packages and internal domain packages.

## Design principles

- **Provider-agnostic domain**
  - Same domain models for logs, metrics, traces, APM and recommendations.
- **Dependency direction**
  - Inbound/outbound adapters depend on core; core never depends on adapters.
- **Configurable and hot-reloadable**
  - Admin-configured providers can be changed without restarting the process.
- **Asynchronous processing**
  - Analysis jobs are queued; result generation can be decoupled from request path.
- **Policy-based outputs**
  - Severity/recommendation behavior is centralized in domain policies.

## Layer layout

```text
           ┌──────────────────────────────┐
           │        Adapters (Inbound)    │  HTTP handlers, DTOs, middlewares
           └──────────────┬───────────────┘
                          │
           ┌──────────────▼───────────────┐
           │      Use Cases (Core)         │  analysis, chat, trace, retention...
           └──────────────┬───────────────┘
                          │
           ┌──────────────▼───────────────┐
           │      Domain + Ports           │  signals, policies, contracts
           └──────────────▲───────────────┘
                          │
           ┌──────────────┴───────────────┐
           │        Adapters (Outbound)    │  collectors, generators, repos, queues
           └──────────────────────────────┘

Cross-cutting platform:
internal/platform/{config,health,logger,telemetry,retry,server,crypto,observability}
```

## Package map

- `cmd/observai-api`: process composition root.
- `internal/core/domain`: normalized domain models (analysis, evidence, chat, audit, auth).
- `internal/core/ports`: abstractions consumed by core.
- `internal/core/usecase`: orchestration use cases (analysis, chat, auth, webhooks, setup, etc.).
- `internal/core/policy`: severity, recommendation and redaction policies.
- `internal/adapters/inbound/http`: HTTP transport, middleware, OpenAPI docs, DTOs.
- `internal/adapters/outbound`:
  - `factory`, `dynamic`, `composite`, `credentials`: adapter composition and dynamic swap support.
  - `prometheus`, `loki`, `elasticsearch`, `jaeger`, `otel`, `datadog`, `dynatrace`, `newrelic`.
  - `ollama`, `openai`, `anthropic`: LLM providers.
  - `postgres`, `redis`, `inmemory`, `asynq`: persistence and queue adapters.
  - `webhooks`, `uuid`, `prompts`, `providertest`.
- `internal/platform`: server bootstrapping, config, tracing, health checks, telemetry, crypto.
- `agents`: LLM runtime prompts and output schemas.

## Request flow: submit analysis

1. `POST /v1/analyses` is validated and persisted as an analysis job (`pending`).
2. A background worker dequeues the job and executes `analysis.RunAnalysisJob`.
3. `usecase.Analysis.executeAnalyze`:
   - validates request
   - collects evidence via `SignalCollector`
   - normalizes and filters evidence
   - applies redaction policy
   - invokes `AnalysisGenerator` (LLM)
   - applies severity/recommendation policies
   - persists result and updates cache/context.
4. Optional notifier emits webhook events (`success`, `failure`, `canceled`).
5. Client consumes:
   - `GET /v1/jobs/{jobID}` for progress.
   - `GET /v1/analyses/{analysisID}` for final result.

## Chat flow

1. Frontend requests `POST /v1/analyses/{id}/chat`.
2. `chat.Ask` validates question scope (in-scope only).
3. It loads analysis context from cache or repository.
4. Calls `ChatResponder` (LLM) and persists the exchange.
5. Chat lock is acquired per-analysis so concurrent questions on the same analysis
   stay serialized in order.

## Async queue and workers

`analysis.WithAsyncBackend()` receives:

- `ports.AnalysisJobRepository` for job state.
- `ports.JobEnqueuer` for enqueue.

Queue implementation is selected by configuration:

- `legacy` (in-memory or Redis queue worker depending on dependencies).
- `asynq` when Redis queue backend is enabled.

Queue-related env vars:

- `OBSERVAI_QUEUE_BACKEND`
- `OBSERVAI_QUEUE_CONCURRENCY`
- `OBSERVAI_QUEUE_DEQUEUE_TIMEOUT`
- `OBSERVAI_CHAT_LOCK_TTL`
- `OBSERVAI_CHAT_LOCK_WAIT`

## Provider loading and reload

At startup, provider and LLM factories are created from configuration.

When admin provider/LLM records exist in DB, reload hooks can rebuild and swap
the runtime adapters atomically without a process restart.

Important consequences:

- New provider configuration is activated dynamically.
- The API remains on previous adapters if a bad configuration would fail to build.
- Health probes and capability payloads always reflect runtime registrations.

## Response contract

All successful responses follow:

```json
{
  "data": { "...": "endpoint payload" },
  "metadata": {
    "requestId": "uuid",
    "processingTimeMs": 0,
    "provider": {
      "mode": "prod",
      "observability": ["prometheus", "loki"],
      "llm": "ollama"
    },
    "warnings": []
  }
}
```

See `contract/README.md` for exact schema and error codes.

## Runtime contracts and operational endpoints

- `GET /health`: lightweight liveness response.
- `GET /healthz`: Kubernetes liveness.
- `GET /readyz`: readiness and dependency probes.
- `GET /metrics`: Prometheus metrics.
- `GET /v1/openapi.yaml`: embedded OpenAPI 3.1 contract.
- `GET /v1/capabilities`: non-sensitive active provider capabilities.

## Security and transport

- Authentication supports:
  - API key bearer (`Authorization: Bearer`),
  - user sessions with JWT cookies (`oai_session`, `oai_refresh`, `oai_csrf`).
- Role and scope validation is centralized in middleware.
- CSRF is enforced on state-changing requests only for browser sessions.
- Provider credentials are loaded from secure references, never from raw provider DTO
  leakage.

## Operational concerns

- `observability` adapter metrics are tagged per provider and operation.
- Retries use bounded exponential backoff + jitter for outbound calls.
- Health and readiness probes include critical dependencies (DB, Redis, providers).
- OpenTelemetry tracer can be enabled with `OBSERVAI_OTEL_EXPORTER_OTLP_ENDPOINT`.

## Tests and reliability

- Unit and integration tests exercise core policies and adapters.
- Integration test suite includes HTTP contract coverage and async analysis path.
- Core use cases are tested with fake/in-memory implementations to keep behavior
  deterministic when external dependencies are missing.

