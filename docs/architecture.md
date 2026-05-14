# Architecture

ObservAI API is an open-source, self-hosted observability analysis gateway
built in Go. It connects to observability providers, collects logs, metrics,
traces and APM data, normalizes those signals and uses configurable LLM
providers to generate technical diagnoses, root cause hypotheses,
recommendations and code-level improvement suggestions.

The platform stays provider-agnostic for both observability sources and LLM
providers. Swapping providers is an adapter change — never a core change.

## Hexagonal layout

```
       ┌────────────────────────────┐
       │     inbound adapters       │  HTTP (chi) — internal/adapters/inbound/http
       └─────────────┬──────────────┘
                     │
       ┌─────────────▼──────────────┐
       │         use cases          │  Analysis, Chat — internal/core/usecase
       └─────────────┬──────────────┘
                     │
       ┌─────────────▼──────────────┐
       │       ports + domain       │  internal/core/{ports,domain,policy}
       └─────────────▲──────────────┘
                     │
       ┌─────────────┴──────────────┐
       │     outbound adapters      │  internal/adapters/outbound/{postgres,redis,
       │                            │     prometheus,ollama,prompts,fake}
       └────────────────────────────┘

       Cross-cutting platform: internal/platform/{config,health,logger,
       observability,retry,server,telemetry}
```

The core never imports framework, SDK or transport packages. Adapters depend
on the core; the core defines the contracts.

## Package layout

```
cmd/observai-api/                  composition root (main, providers wiring)
internal/core/domain               normalized models (Analysis, Evidence, Chat*)
internal/core/ports                contracts the core depends on
internal/core/usecase              orchestration (Analysis, Chat)
internal/core/policy               severity/recommendation policy objects
internal/adapters/inbound/http     chi router, DTOs, error mapper, OpenAPI
internal/adapters/outbound/fake    deterministic in-memory implementations
internal/adapters/outbound/postgres  pgx + sqlc adapter for AnalysisRepository
                                     and ChatHistoryRepository
internal/adapters/outbound/redis     analysis context cache (TTL-bounded)
internal/adapters/outbound/prometheus signal collector (PromQL)
internal/adapters/outbound/ollama    analysis generator and chat responder
internal/adapters/outbound/prompts   versioned LLM prompt loader
internal/platform/config           cleanenv-backed configuration
internal/platform/health           health checker + probes
internal/platform/logger           slog wiring + per-request logger
internal/platform/observability    provider metrics observer
internal/platform/retry            bounded exponential backoff + full jitter
internal/platform/server           net/http server with graceful shutdown
internal/platform/telemetry        OpenTelemetry + Prometheus client
migrations/                         golang-migrate SQL files
agents/                             public, versioned LLM prompts
api/                                API contract overview, points at openapi.yaml
docs/                               human-readable architecture and decisions
```

## Ports and adapters

| Port (in `internal/core/ports`) | Responsibility | Implementations |
| --- | --- | --- |
| `AnalysisRepository` | Persist and query analyses | `postgres.AnalysisRepository`, `fake.AnalysisRepository` |
| `ChatHistoryRepository` | Persist and list chat exchanges | `postgres.AnalysisRepository`, `fake.AnalysisRepository` |
| `SignalCollector` | Pull normalized evidence from observability providers | `prometheus.SignalCollector`, `fake.SignalCollector` |
| `AnalysisGenerator` | Produce structured diagnoses from evidence via an LLM | `ollama.AnalysisGenerator`, `fake.AnalysisGenerator` |
| `ChatResponder` | Answer scoped follow-up questions via an LLM | `ollama.ChatResponder`, `fake.ChatResponder` |
| `AnalysisContextCache` | TTL-bounded cache of analysis context for chat | `redis.AnalysisContextCache`, `fake.AnalysisContextCache` |
| `IDGenerator` | Assign analysis identifiers | `fake.IDGenerator` |

Each adapter owns the SDK types, error format and retry behaviour of its
provider. Adapters never leak SDK fields into the core.

## Request flow: `POST /v1/analyses`

1. **HTTP entry** — `internal/adapters/inbound/http/router.go:99` (`handleCreateAnalysis`)
   decodes `AnalysisRequestDto`, runs go-playground validation, maps to
   `domain.AnalysisRequest`.
2. **Orchestration** — `internal/core/usecase/analysis.go:60` (`Analyze`):
   collects evidence, caps it at 25 items for the LLM call, invokes
   `AnalysisGenerator`, reconciles severity and recommendations through
   `policy.SeverityPolicy` and `policy.RecommendationPolicy`, saves the
   result and caches the analysis context.
3. **Persistence** — `internal/adapters/outbound/postgres/analysis_repository.go:67`
   (`Save`) calls the sqlc-generated query and serializes nested fields as
   `jsonb`.
4. **Response shaping** — `internal/adapters/inbound/http/router.go:233`
   (`writeSuccess`) wraps the response with `WrapperDtoResponde` and the
   `ResponseMetadata` envelope.

The same shape applies to chat (`handleChat → Chat.Ask →
chatScopePolicy → ChatResponder → ChatHistoryRepository → writeSuccess`).
Chat scope is enforced **before** the responder runs; see
`internal/core/usecase/chat_scope_policy.go`.

## Public response contract

Every successful response is wrapped by:

```jsonc
{
  "data": { /* endpoint-specific payload */ },
  "metadata": {
    "requestId": "uuid",
    "processingTimeMs": 12,
    "provider": { "mode": "local", "observability": ["fake"], "llm": "fake" },
    "warnings": [],
    "pagination": { "limit": 20, "offset": 0, "next": "/v1/analyses?limit=20&offset=20" }
  }
}
```

Error responses use the same wrapper with `ErrorResponse` inside `data`. The
machine-readable `code` values are defined in
[`internal/adapters/inbound/http/error_mapper.go`](../internal/adapters/inbound/http/error_mapper.go)
and listed in the OpenAPI spec.

The full contract lives in
[`internal/adapters/inbound/http/openapi.yaml`](../internal/adapters/inbound/http/openapi.yaml)
and is served by the running binary at `GET /v1/openapi.yaml`. See
[`api/README.md`](../api/README.md) for editing rules.

## Cross-cutting patterns

- **Retry with full jitter**: `internal/platform/retry` is shared by Ollama and
  Prometheus clients (3 attempts, 100ms base, 2s cap). Each adapter classifies
  its own transient errors so retries stay type-safe.
- **Null objects**: `internal/core/usecase/noop_dependencies.go` provides a
  no-op `AnalysisContextCache` so the use case stays stable when the cache is
  unconfigured.
- **Policy objects**: severity and recommendation reconciliation live in
  `internal/core/policy/` — the use case orchestrates them but never branches
  on provider-specific data.
- **Strategy / fake-vs-real selection**: `cmd/observai-api/providers.go` and
  `cmd/observai-api/adapters.go` choose real or deterministic-fake adapters
  based on configuration. Use cases stay unaware of the choice.
- **Chat scope policy**: a composite policy enforces keyword matching plus
  refusal-keyword detection inside the core, isolating chat guardrails from
  transport and adapter concerns.

For the rules these patterns enforce, see
[`.claude/rules/architecture.md`](../.claude/rules/architecture.md) and
[`.claude/rules/pattern-strategy.md`](../.claude/rules/pattern-strategy.md).

## Observability and operational endpoints

- `GET /health` — application liveness (lightweight).
- `GET /healthz` — Kubernetes liveness probe.
- `GET /readyz` — readiness; reflects critical dependencies.
- `GET /metrics` — Prometheus exposition (`internal/platform/telemetry`).
- `GET /v1/openapi.yaml` — embedded OpenAPI 3.1 document.

Logs are structured slog; tracing is OpenTelemetry with context propagation
across inbound HTTP, use cases, outbound adapters and database calls.

## Runtime LLM prompts

LLM behaviour is defined by versioned files under `agents/`:

- `agents/observability-analysis-agent.md` — system prompt + JSON output schema
  consumed by `internal/adapters/outbound/ollama/analysis_generator.go`.
- `agents/interaction-chat-agent.md` — chat scope, refusal envelope and JSON
  schema consumed by `internal/adapters/outbound/ollama/chat_responder.go`.
- `agents/prompt-translator-agent.md` — translates user intent into compact
  analysis requests.

Files are loaded through `internal/adapters/outbound/prompts.FileLoader`,
which caches results per file name. `.claude/` and `CLAUDE.md` are local
working instructions and never feed runtime behaviour.

## Tests

- **Unit tests** live next to each package and are exercised by
  `go test ./...`. They never require Docker, Postgres, Redis or network
  access.
- **Postgres adapter integration tests** at
  `internal/adapters/outbound/postgres/analysis_repository_integration_test.go`
  exercise the repository in isolation. They require an external Postgres
  instance via `OBSERVAI_TEST_DATABASE_DSN`.
- **End-to-end HTTP integration test** at
  `internal/adapters/inbound/http/integration_test.go` (build tag
  `integration`) spins up a real PostgreSQL container via `testcontainers-go`,
  applies migrations with `golang-migrate` and drives `POST /v1/analyses`,
  `GET /v1/analyses/{id}`, `GET /v1/analyses`, `POST /v1/analyses/{id}/chat`,
  `GET /v1/analyses/{id}/chat` and the out-of-scope rejection path.

  Run it with:

  ```bash
  GOCACHE=/tmp/observai-go-build-cache \
    go test -tags=integration -count=1 -timeout=5m \
    ./internal/adapters/inbound/http/...
  ```

  Set `OBSERVAI_TEST_TESTCONTAINERS=0` to skip when Docker is unavailable.

## Boundary rules

- The domain (`internal/core/domain`) never imports adapter packages, SDKs,
  HTTP, database or cache clients.
- HTTP DTOs (`internal/adapters/inbound/http/dto.go`) never become domain
  entities. Mapping functions convert at the edge.
- Postgres queries live behind sqlc-generated code under
  `internal/adapters/outbound/postgres`. The use case talks to the
  `AnalysisRepository` interface.
- Provider SDK error types stay inside their adapter package; the core sees
  sentinel domain errors (`domain.ErrAnalysisNotFound`,
  `domain.ErrInvalidAnalysisRequest`, `domain.ErrQuestionOutOfScope`, etc.).
