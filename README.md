<div align="center">

# ObservAI API

**Open-source AI gateway for observability analysis across logs, metrics, traces and APM providers.**

Bring your own APM. Bring your own LLM. Analyze everything locally.

<img src="https://img.shields.io/badge/Go-00ADD8?style=for-the-badge&logo=go&logoColor=white" />
<img src="https://img.shields.io/badge/Observability-2563EB?style=for-the-badge" />
<img src="https://img.shields.io/badge/AI_Agnostic-7C3AED?style=for-the-badge" />
<img src="https://img.shields.io/badge/Self_Hosted-0F172A?style=for-the-badge" />
<img src="https://img.shields.io/badge/License-MIT-green?style=for-the-badge" />

</div>

---

## About

**ObservAI API** is the backend core of ObservAI, an open-source and self-hosted platform for intelligent observability analysis.

The API connects to observability providers, collects logs, metrics, traces and APM data, normalizes those signals and uses a configurable LLM provider to generate technical insights, root cause hypotheses, performance recommendations and code-level improvement suggestions.

ObservAI does not replace tools like Dynatrace, Datadog, Elasticsearch, Prometheus, Jaeger, Loki or Grafana. It works as an intelligent analysis layer on top of them.

The goal is to help engineers investigate production behavior faster and transform raw observability signals into actionable technical decisions.

---

## What ObservAI API does

ObservAI API orchestrates the full analysis flow:

```txt
User request
   ↓
Analysis API
   ↓
Provider connectors
   ↓
Logs + Metrics + Traces + APM data
   ↓
Signal normalization
   ↓
Context builder
   ↓
LLM provider
   ↓
Structured diagnosis
   ↓
Interactive AI chat context
```

The API is designed to analyze:

- Application logs
- Metrics
- Distributed traces
- APM events
- Error spikes
- Latency increases
- Slow spans
- Expensive operations
- Dependency bottlenecks
- Database bottlenecks
- Infrastructure saturation
- Anomalies
- Incident symptoms
- Root cause hypotheses
- Code-level improvement opportunities

---

## Core capabilities

### Logs analysis

Collect and analyze logs from providers such as Elasticsearch, OpenSearch, Loki, Datadog or Dynatrace.

ObservAI can identify repeated exceptions, error patterns, suspicious messages, incident timelines and correlations between logs, metrics and traces.

### Metrics analysis

Inspect metrics from providers such as Prometheus, Datadog, Dynatrace and other compatible systems.

ObservAI can analyze CPU, memory, latency, throughput, error rate, saturation, dependency latency and custom business metrics.

### Trace analysis

Analyze distributed traces from Jaeger, Dynatrace, Datadog, OpenTelemetry-compatible backends and other tracing providers.

ObservAI can inspect spans, duration, execution paths, service dependencies and request flow to identify where a request spends most of its time.

It can help detect:

- Slow spans
- Expensive operations
- N+1 query patterns
- Slow database calls
- Excessive network hops
- External API bottlenecks
- Long synchronous chains
- Retry amplification
- Inefficient service orchestration
- Possible code-level performance problems

Example:

```txt
Analyze this distributed trace and identify the main performance bottlenecks.
Suggest improvements at service, database and code level.
```

### APM analysis

Connect to APM providers and analyze application health, service behavior, dependencies, error rates and performance trends.

Supported provider types include Dynatrace, Datadog, New Relic, Elastic APM, OpenTelemetry-compatible systems and custom adapters.

### AI-generated technical diagnosis

Each analysis produces a structured, actionable result:

```json
{
  "summary": "checkout-service is experiencing increased latency and error rate.",
  "severity": "high",
  "affectedServices": ["checkout-service", "payment-service"],
  "detectedAnomalies": [
    "p95 latency increased by 430%",
    "payment authorization span became the dominant latency source"
  ],
  "possibleRootCauses": [
    "External payment provider degradation",
    "Inefficient retry strategy",
    "Possible N+1 query pattern"
  ],
  "codeLevelInsights": [
    "Review synchronous dependency calls inside checkout flow",
    "Add timeout, circuit breaker and fallback around payment authorization"
  ],
  "recommendedActions": [
    "Inspect recent deployments",
    "Analyze payment provider response time",
    "Review database query execution"
  ]
}
```

---

## Integrated AI chat

ObservAI API provides the backend foundation for a context-aware AI chat.

After an analysis is completed, the user can continue the investigation through a conversational interface. The chat uses the generated analysis, collected evidence, detected anomalies and provider data as context.

Example questions:

```txt
What should I fix first?
Can this be related to the last deployment?
Which service is the main bottleneck?
Is there any evidence of database saturation?
Explain this trace in simple terms.
Can you suggest code-level improvements?
```

---

## Bring your own LLM

ObservAI API is AI-provider agnostic.

Supported AI provider types (shipped adapter):

- OpenAI
- Anthropic
- Azure OpenAI (via the `openai-compatible` alias)
- OpenRouter (via the `openai-compatible` alias)
- Ollama
- LM Studio and other local/self-hosted backends that speak the OpenAI API (via the `openai-compatible` alias)

Roadmap (no adapter yet, contributions welcome):

- Gemini

The user owns the token, the provider and the data flow.

---

## Bring your own observability stack

ObservAI API is observability-provider agnostic.

| Provider | Logs | Metrics | Traces | APM |
|---|---:|---:|---:|---:|
| Elasticsearch | ✅ | ❌ | ❌ | ⚠️ |
| OpenSearch | ✅ | ❌ | ❌ | ⚠️ |
| Loki | ✅ | ❌ | ❌ | ❌ |
| Prometheus | ❌ | ✅ | ❌ | ❌ |
| Jaeger | ❌ | ❌ | ✅ | ❌ |
| Dynatrace | ✅ | ✅ | ✅ | ✅ |
| Datadog | ✅ | ✅ | ✅ | ✅ |
| New Relic | ✅ | ✅ | ✅ | ✅ |
| OpenTelemetry | ⚠️ | ✅ | ✅ | ⚠️ |

---

## Configuration

ObservAI is configured in two stages.

Bootstrap configuration comes from environment variables and is limited to the
settings required for the API to start: mode, port, database, Redis, encryption,
migrations and operational guards.

Application configuration is managed after startup through the admin interface
or admin API: observability providers, LLM providers, users, API keys, webhooks,
retention and other runtime resources.

Minimum local bootstrap:

```bash
OBSERVAI_API_PORT=8080 \
OBSERVAI_ENV=local \
OBSERVAI_MODE=local \
OBSERVAI_TIMEZONE=Local \
OBSERVAI_DATABASE_DSN=postgres://observai:observai@localhost:5432/observai?sslmode=disable \
OBSERVAI_REDIS_URL=redis://localhost:6379/0 \
OBSERVAI_ENCRYPTION_KEY=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef \
OBSERVAI_JWT_SECRET=local-development-jwt-secret-change-before-production \
go run ./cmd/observai-api
```

Use `.env.example` as the reference for local bootstrap variables:

```env
OBSERVAI_API_PORT=8080
OBSERVAI_ENV=local
OBSERVAI_MODE=local
OBSERVAI_TIMEZONE=Local
OBSERVAI_DATABASE_DSN=postgres://observai:observai@localhost:5432/observai?sslmode=disable
OBSERVAI_REDIS_URL=redis://localhost:6379/0
OBSERVAI_ENCRYPTION_KEY=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
OBSERVAI_JWT_SECRET=local-development-jwt-secret-change-before-production
OBSERVAI_ANALYSIS_CONTEXT_CACHE_TTL=6h
OBSERVAI_QUEUE_CONCURRENCY=5
OBSERVAI_QUEUE_DEQUEUE_TIMEOUT=5s
OBSERVAI_CHAT_LOCK_TTL=60s
OBSERVAI_CHAT_LOCK_WAIT=30s
```

When a `.env` file exists in the API working directory, it is loaded on
startup before the environment is read. Already exported variables still take
precedence over values from `.env`.

The Docker image sets `TZ=America/Sao_Paulo` by default so `OBSERVAI_TIMEZONE=Local`
uses the container timezone. Build another default with
`--build-arg DEFAULT_TIMEZONE=Etc/UTC`, or override `TZ`/`OBSERVAI_TIMEZONE`
at runtime for another deployment region.

## Async analysis and concurrent chat

Analyses run asynchronously to keep request latency low and the LLM provider under
backpressure even when many users hit the API at the same time.

```txt
POST /v1/analyses
  -> 202 Accepted, body: { jobId, status, statusUrl }, header: Location: /v1/jobs/{jobId}

GET /v1/jobs/{jobId}
  -> 200 with { status: pending|running|completed|failed, analysisId? }

GET /v1/analyses/{analysisId}
  -> 200 with the final analysis once the job is completed
```

The same identifier flows through the lifecycle: `jobId` and `analysisId` are
equal once the worker finishes. `OBSERVAI_QUEUE_CONCURRENCY` caps how many
analyses execute in parallel on a single instance.

Chat (`POST /v1/analyses/{id}/chat`) stays synchronous from the client's point
of view but is serialized per analysis through a Redis lock (in-process mutex
when Redis is not configured). Concurrent questions about the same analysis
hit the LLM in FIFO order; questions about different analyses run in parallel.
See `docs/architecture/concurrency.md` for the full model.

## Run locally with live reload

Air is registered as a Go tool and uses `.air.toml` from the repository root.

```bash
go tool air
```

By default, the API runs in local mode with in-memory fallbacks when external
dependencies are not configured. To run Air against local Postgres/Redis, create
`.env` from `.env.example` first:

```bash
cp .env.example .env
go tool air
```


---

## Database migrations

Migrations use golang-migrate and live in `migrations/`.

```bash
migrate -path migrations -database "$OBSERVAI_DATABASE_DSN" up
```

SQL queries prepared for sqlc live under `internal/adapters/outbound/postgres/query`.

---

## Container deployment sketch

The repository intentionally does not ship a root `docker-compose.yml`.
Deployment topology depends on the operator's database, cache, observability
providers, LLM providers, ingress and secret-management choices. The snippet
below is only a minimal container wiring reference for the API plus its core
Postgres/Redis dependencies.

```yaml
services:
  observai-api:
    image: observai/observai-api:latest
    container_name: observai-api
    ports:
      - "8080:8080"
    env_file:
      - .env
    environment:
      TZ: America/Sao_Paulo
    depends_on:
      - postgres
      - redis

  postgres:
    image: postgres:16-alpine
    container_name: observai-postgres
    environment:
      POSTGRES_USER: observai
      POSTGRES_PASSWORD: observai
      POSTGRES_DB: observai
    volumes:
      - observai_postgres_data:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine
    container_name: observai-redis
    volumes:
      - observai_redis_data:/data

volumes:
  observai_postgres_data:
  observai_redis_data:
```

---

## Security principles

Recommended practices:

- Use environment variables or secret managers
- Avoid committing tokens
- Encrypt stored credentials
- Restrict access to the web interface
- Run behind an internal network, VPN or private ingress
- Use HTTPS in production
- Prefer read-only provider credentials whenever possible

---

## License

This project is licensed under the MIT License.
