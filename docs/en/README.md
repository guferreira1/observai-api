# ObservAI API Documentation

ObservAI API is an open-source, self-hosted backend for observability analysis.  
It is not a visualization dashboard: it is an analysis engine that connects to
observability systems, normalizes data, and produces AI-assisted technical
findings.

The project is implemented in **Go** with a **hexagonal architecture**, so the
core domain is isolated from external concerns such as HTTP frameworks, SDKs,
databases, queues, and LLM providers.

## What this backend provides

- **Asynchronous analyses** over logs, metrics, traces, and APM data.
- **Provider-agnostic design** for observability backends and LLM providers.
- **Policy-based analysis output** with severity, confidence, hypotheses, code-level
  recommendations, and evidence references.
- **Contextual chat** attached to each analysis, with scope validation and
  conversation history.
- **Admin plane** for provider configuration, API keys, users, webhooks, and audit logs.
- **Operational endpoints** for health, readiness, metrics, and runtime
  capability discovery.
- **Deterministic tests** with explicit ports and in-memory alternatives.

## Recommended reading

- [English](architecture.md)
- [Portuguese](../pt-br/README.md)

## Core architecture in one view

```txt
HTTP client -> inbound HTTP adapters -> use cases -> domain/policies
                         ^            |
                         |            +-> outbound adapters (LLM + providers + storage + queues)
                         +-> OpenAPI contract + authentication + metrics + tracing
```

## Quick start

1. Prepare configuration (minimum local mode example):

```bash
OBSERVAI_API_PORT=8080 \
OBSERVAI_ENV=local \
OBSERVAI_MODE=local \
OBSERVAI_DATABASE_DSN=postgres://observai:observai@localhost:5432/observai?sslmode=disable \
OBSERVAI_REDIS_URL=redis://localhost:6379/0 \
OBSERVAI_ENCRYPTION_KEY=change-me-at-least-32-characters-long \
go run ./cmd/observai-api
```

2. Submit an analysis:

```bash
curl -X POST http://localhost:8080/v1/analyses \
  -H "Authorization: Bearer <API_KEY_OR_COOKIE>" \
  -H "Content-Type: application/json" \
  -d '{
    "goal": "Investigate high latency in checkout during the last 30 minutes",
    "timeWindow": {"start":"2026-05-15T09:00:00Z","end":"2026-05-15T09:30:00Z"},
    "affectedServices": ["checkout-api"],
    "signals": ["metrics","logs","traces"],
    "context": "Deploy at 08:45 UTC"
  }'
```

3. Poll the returned job:

```bash
curl http://localhost:8080/v1/jobs/<jobId>
```

4. Fetch analysis result once completed:

```bash
curl http://localhost:8080/v1/analyses/<analysisId>
```

5. Ask follow-up questions about the same analysis:

```bash
curl -X POST http://localhost:8080/v1/analyses/<analysisId>/chat \
  -H "Authorization: Bearer <API_KEY_OR_COOKIE>" \
  -H "X-CSRF-Token: <csrf-from-session-if-browser>" \
  -H "Content-Type: application/json" \
  -d '{ "question": "What is the strongest evidence for this latency?" }'
```

> The browser session flow uses `oai_session`, `oai_refresh`, and `oai_csrf`
> cookies. API-key flow uses `Authorization: Bearer`.

## How components are split

- `cmd/observai-api`: startup composition root, reads config and wires adapters.
- `internal/core/*`: application domain, ports, policies, and use cases.
- `internal/adapters/inbound/http`: HTTP contract, middleware, DTO mapping, OpenAPI.
- `internal/adapters/outbound/*`: integrations and side-effect adapters.
- `internal/platform/*`: cross-cutting concerns (config, telemetry, retries, health,
  logging, server, crypto, migrations).
- `agents/*`: versioned prompts used by LLM adapters.

## Runtime and integration model

- Bootstrap settings come from environment variables so the API can start before
  the admin interface is available.
- Observability providers, LLM providers and other runtime resources are managed
  through the admin interface or admin API after startup.
- Observability and LLM providers are normalized to a provider-agnostic signal
  and result model before entering the core.
- Every successful response follows the `WrapperDtoResponde` contract (`data`,
  `metadata`).
- Asynchronous analysis execution decouples request latency from LLM processing.

## Deployment notes

- For simple local development, in-memory repositories are used when no Postgres
  or Redis is available.
- In production, enable Postgres and Redis and use worker configuration:
  `OBSERVAI_QUEUE_BACKEND`, `OBSERVAI_QUEUE_CONCURRENCY`, and lock settings.
- Observability exports metrics under `/metrics`.

## For API consumers

- Use `GET /v1/openapi.yaml` as machine-readable API contract.
- Read `contract/README.md` in both language directories for envelope and auth
  details.
- Use provider integration docs when configuring `observability` and `llm` entries.

## Related docs

See:

- `architecture.md`
- `contract/README.md`
- `providers/README.md`
- `ROADMAP.md`
