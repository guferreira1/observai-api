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

Supported AI provider types:

- OpenAI
- Anthropic
- Gemini
- Azure OpenAI
- OpenRouter
- Ollama
- LM Studio
- Local/self-hosted LLMs

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

## Architecture

```txt
observai-api
├── cmd/
│   └── api/                  # Application entrypoint
├── internal/
│   ├── analysis/             # Analysis orchestration and diagnosis generation
│   ├── providers/            # Observability provider adapters
│   ├── llm/                  # LLM provider adapters
│   ├── workers/              # Worker pool and concurrent execution
│   ├── queue/                # Queue management
│   ├── sessions/             # Analysis sessions and chat context
│   ├── security/             # Token encryption and auth helpers
│   ├── storage/              # Database access
│   ├── config/               # Configuration loading
│   └── http/                 # Handlers, routes and middleware
├── pkg/
│   └── contracts/            # Public contracts and shared types
├── deployments/
│   ├── docker-compose.yml
│   └── k8s/
└── docs/
    ├── architecture.md
    ├── providers.md
    ├── llm.md
    └── security.md
```

---

## Backend responsibilities

The Go backend is responsible for:

- API gateway
- Provider management
- LLM provider management
- Secure token handling
- Analysis orchestration
- Worker pool execution
- Queue management
- Session isolation
- Context generation
- Result normalization
- Streaming responses
- Analysis history
- Chat context persistence

A key part of the architecture is the worker execution model, which isolates analysis sessions and prevents context leakage between users or concurrent requests.

---

## Environment variables

```env
OBSERVAI_API_PORT=8080
OBSERVAI_ENV=local

POSTGRES_USER=observai
POSTGRES_PASSWORD=observai
POSTGRES_DB=observai
POSTGRES_HOST=postgres
POSTGRES_PORT=5432

REDIS_URL=redis://redis:6379

OBSERVAI_ENCRYPTION_KEY=change-me
OBSERVAI_JWT_SECRET=change-me

OPENAI_API_KEY=
ANTHROPIC_API_KEY=
GEMINI_API_KEY=
OPENROUTER_API_KEY=

OLLAMA_BASE_URL=http://ollama:11434
```

---

## Run with Docker Compose

```bash
docker compose up -d
```

API:

```txt
http://localhost:8080
```

Health check:

```txt
http://localhost:8080/health
```

---

## Example Docker Compose

```yaml
services:
  observai-api:
    image: ghcr.io/guferreira1/observai-api:latest
    container_name: observai-api
    ports:
      - "8080:8080"
    env_file:
      - .env
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
