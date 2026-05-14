# Project decisions

This document records the main technical decisions already defined for ObservAI API.

ObservAI API is an open-source, self-hosted observability analysis gateway built in Go. The API connects to observability providers, collects logs, metrics, traces and APM data, normalizes those signals and uses configurable LLM providers to generate technical diagnoses, root cause hypotheses, performance recommendations and code-level improvement suggestions.

The project must remain provider-agnostic for both observability sources and LLM providers.

---

## Language decision

ObservAI API will be built in Go.

Go was chosen because the project needs high performance, low memory overhead, strong concurrency support, simple deployment, good support for backend services and strong fit for infrastructure, observability and platform tooling.

The project should prioritize idiomatic Go over abstractions copied from other ecosystems such as Java, Spring, Node.js or NestJS.

---

## Architecture decision

The project will follow hexagonal architecture.

The core must contain domain models, value objects, use cases, ports, business rules, normalization contracts and analysis contracts.

Adapters must contain external details such as HTTP, databases, cache, queues, observability providers, LLM providers, third-party SDKs and infrastructure integrations.

The core must not depend on frameworks, provider SDKs, database clients, HTTP routers or transport details.

Dependency direction:

    inbound adapters -> use cases -> ports/domain <- outbound adapters

The core defines ports.

Adapters implement ports.

---

## Provider-agnostic design decision

ObservAI API must be provider-agnostic.

The same core flow must work regardless of the observability or LLM provider.

Only adapters should change when adding a new provider.

Examples of observability provider adapters:

- Dynatrace
- Datadog
- New Relic
- Elastic APM
- Elasticsearch
- OpenSearch
- Loki
- Prometheus
- Jaeger
- OpenTelemetry-compatible providers

Examples of LLM provider adapters:

- OpenAI
- Anthropic
- Gemini
- Azure OpenAI
- OpenRouter
- Ollama
- LM Studio
- local/self-hosted models

Provider SDK types must not leak into the core.

Provider responses must be converted into internal normalized models before reaching use cases.

---

## Recommended Go stack decision

The initial recommended stack for ObservAI API is:

- HTTP/router: chi
- Database: PostgreSQL
- PostgreSQL driver: pgx
- SQL access: sqlc
- Migrations: golang-migrate
- Configuration: cleanenv or envconfig
- Logging: slog
- Metrics: Prometheus client
- Tracing: OpenTelemetry
- Tests: testing, testify, httptest
- Validation: go-playground/validator
- Redis: go-redis
- Dependency injection: manual constructors initially
- Future dependency injection option: wire
- API documentation: oapi-codegen or swaggo

The project should avoid large frameworks unless there is a strong reason.

Prefer:

    standard library + small focused libraries + explicit architecture

---

## HTTP decision

The preferred HTTP router is chi.

Reason:

- works directly with net/http
- is lightweight
- does not force a heavy framework style
- fits well with hexagonal architecture
- keeps handlers simple
- makes it easier to isolate inbound adapters from core use cases
- works well with standard Go middleware
- works well with OpenTelemetry instrumentation

Gin is acceptable for faster productivity, but chi is preferred for this project.

Fiber should be avoided initially because it does not use net/http directly and may reduce compatibility with standard Go middleware and instrumentation.

---

## Database decision

The preferred database approach is:

    PostgreSQL + pgx + sqlc

Reason:

- keeps SQL explicit
- generates type-safe Go code
- avoids heavy ORM behavior
- improves performance and predictability
- fits well with clean and hexagonal architecture
- avoids hiding important query behavior
- makes database access easier to test and review

GORM should be avoided initially unless there is a strong productivity reason.

Ent can be considered later if the domain model becomes large and schema-driven development becomes useful.

---

## Migration decision

Database migrations should use golang-migrate.

Reason:

- mature ecosystem
- simple migration files
- works well in CI/CD
- works well with Docker and Kubernetes jobs
- keeps schema evolution explicit

Migration files must be versioned and reviewed carefully.

---

## Logging decision

The initial logging library should be slog.

Reason:

- it is part of the Go standard library
- supports structured logging
- avoids unnecessary external dependency
- is enough for the first version of the API
- integrates well with common observability pipelines

Zap can be considered later if benchmarks show that logging performance became a real bottleneck.

Logs must be structured.

Logs must not expose secrets, provider tokens or sensitive payloads.

---

## Observability decision

ObservAI API must be observable by design.

The base stack should include:

    OpenTelemetry + Prometheus + structured logs

The API should expose metrics for:

- HTTP request count
- HTTP request latency
- HTTP error count
- provider call latency
- provider call failures
- LLM call latency
- LLM call failures
- background worker backlog, when applicable
- cache hit/miss ratio, when applicable
- analysis execution time
- analysis failures

Tracing must propagate context across inbound HTTP requests, use cases, outbound provider adapters, database calls, cache calls, LLM calls and background workers.

ObservAI analyzes observability data, so the API itself must also be easy to observe.

---

## Configuration decision

Configuration should be loaded from environment variables.

Recommended libraries:

- cleanenv
- envconfig

Viper should only be used if the project needs multiple configuration sources such as YAML, JSON, environment variables and runtime config files.

For the first version, keep configuration simple.

Configuration structs must be explicit.

Invalid or missing required configuration should fail fast during startup.

---

## Dependency injection decision

Dependency injection should start manually through constructor functions.

Example:

    func NewAnalyzeUseCase(
        repository AnalysisRepository,
        provider ObservabilityProvider,
        llm LLMProvider,
    ) *AnalyzeUseCase

Manual DI is preferred initially because it is simple, explicit, idiomatic in Go, easy to test, easy to understand and free from runtime magic.

wire can be adopted later if the dependency graph grows too much.

Avoid runtime-heavy dependency injection frameworks at the beginning.

---

## Testing decision

The project should use:

- testing as the base test package
- testify for assertions when it improves readability
- httptest for HTTP handlers
- table-driven tests when useful

Use cases must have unit tests.

Adapters must have tests when they contain mapping logic, normalization logic, error handling, provider-specific transformation, retry behavior or timeout behavior.

Tests must not depend on real provider credentials.

Tests must be deterministic.

Tests must validate behavior, not private implementation details.

---

## Comment decision

The project must not use ordinary implementation comments.

Allowed comments:

- GoDocs for exported structs
- GoDocs for exported interfaces
- GoDocs for exported functions
- GoDocs for exported constants
- package documentation when useful

Do not add comments explaining obvious implementation details.

Prefer clear names, small functions, readable code, cohesive packages and explicit behavior.

---

## Security decision

No real secret, token, password, API key, private key or sensitive credential may be committed.

Provider credentials must come from environment variables, secret managers, local ignored files or deployment secrets.

Logs must never expose credentials.

Provider errors must be sanitized before being returned through API responses.

LLM payloads must be built carefully because logs, traces and APM events may contain sensitive data.

The API must prefer data minimization before sending context to LLM providers.

Sensitive data must not be stored unless strictly necessary.

---

## LLM provider decision

ObservAI API must support bring-your-own-LLM.

The API must not be tied to a single LLM provider.

Supported provider types may include:

- OpenAI
- Anthropic
- Gemini
- Azure OpenAI
- OpenRouter
- Ollama
- LM Studio
- local/self-hosted models

The selected provider must be explicit in configuration or request flow.

The system must not silently forward data to another provider.

The LLM adapter must be isolated from the core.

The core must depend on an LLM port, not on provider SDKs.

---

## Observability provider decision

ObservAI API must support bring-your-own-observability-stack.

Supported provider types may include:

- Dynatrace
- Datadog
- New Relic
- Elastic APM
- Elasticsearch
- OpenSearch
- Loki
- Prometheus
- Jaeger
- OpenTelemetry-compatible systems

Each provider must be implemented as an adapter.

The core must operate over normalized signals, not raw provider payloads.

---

## Normalized signal decision

The core should work with normalized internal models for:

- logs
- metrics
- traces
- spans
- APM events
- anomalies
- service dependencies
- incident evidence
- analysis results

Provider-specific fields may exist in metadata, but the main use cases must depend on stable internal models.

Normalized models should make it possible to correlate logs with traces, traces with metrics, metrics with incidents, APM data with service health and deployment events with failures when available.

---

## Analysis result decision

Analysis results should be structured and actionable.

A diagnosis should prefer a format close to:

    {
      "summary": "Short technical summary",
      "severity": "low | medium | high | critical",
      "affectedServices": [],
      "detectedAnomalies": [],
      "possibleRootCauses": [],
      "evidence": [],
      "codeLevelInsights": [],
      "recommendedActions": []
    }

The result must contain evidence whenever possible.

Avoid returning only generic AI text.

The result should help engineers decide what to inspect or fix first.

Recommendations must be practical and technically grounded.

---

## Performance decision

The API must be designed for high-volume observability data.

Important rules:

- avoid unnecessary large payload copies
- use pagination or streaming when needed
- define timeouts for provider calls
- limit concurrency explicitly
- avoid unbounded goroutines
- avoid loading huge provider responses into memory without control
- measure provider and LLM latency
- prefer bounded queues
- avoid uncontrolled fan-out
- avoid processing all provider data when a smaller time window is enough

Concurrency must be intentional, bounded and observable.

---

## Reliability decision

External calls must have timeout, context propagation, error handling, structured logs, metrics and predictable failure behavior.

The API should support graceful shutdown.

Health checks should include liveness, readiness and dependency readiness when applicable.

Provider failures should not crash the whole service unless the failed dependency is required for startup.

LLM failures should return clear errors and should not corrupt analysis state.

---

## Cache decision

Redis can be used for:

- temporary analysis context
- session state
- async job status
- provider response cache
- rate limiting
- short-lived correlation data

The recommended Redis client is:

    github.com/redis/go-redis

Cache usage must define key format, TTL, invalidation behavior, serialization format and error fallback behavior.

The core must not depend directly on Redis.

Redis must be accessed through ports and outbound adapters.

---

## Async processing decision

The first version should validate the synchronous analysis flow before introducing complex async processing.

Async processing can be introduced when needed for long-running provider analysis, large trace analysis, batch analysis, scheduled analysis, background enrichment or report generation.

Potential libraries:

- asynq for Redis-backed jobs
- kafka-go for Kafka
- watermill for event-driven abstractions

Avoid introducing queues before the core contracts are stable.

---

## API documentation decision

API documentation may use one of two approaches.

### Option 1: OpenAPI-first

Use oapi-codegen.

Good when the API contract should be designed first and generated into Go types.

### Option 2: Code-first

Use swaggo.

Good when the team wants faster documentation directly from handlers.

For the first version, either is acceptable.

For a more professional public API, oapi-codegen is preferred.

---

## Validation decision

Use go-playground/validator for request validation when validation tags are enough.

Complex business validation must stay in use cases or domain logic.

HTTP validation must not become business rule validation.

Inbound adapters validate transport-level input.

Use cases validate business-level consistency.

---

## Error handling decision

Errors must be explicit.

Use cases should return meaningful domain/application errors.

Adapters should convert external errors into internal errors when crossing boundaries.

API handlers should translate internal errors into proper HTTP responses.

Provider SDK errors must not leak raw details to external API consumers.

Errors should include enough context for logs, but API responses should remain safe.

---

## Package organization decision

The preferred initial structure is:

    cmd/
      observai-api/
        main.go

    internal/
      core/
        domain/
        ports/
        usecase/

      adapters/
        inbound/
          http/
        outbound/
          postgres/
          redis/
          llm/
          observability/

      platform/
        config/
        logger/
        telemetry/
        server/

This structure can evolve, but the dependency direction must remain protected.

---

## Git decision

Agents and automated assistants must not work directly on main.

Every change must happen in a dedicated branch.

The following actions require explicit owner authorization:

- commit
- push
- pull
- merge
- rebase
- force update
- opening pull requests
- changing repository settings

When working locally, agents may inspect, edit, run tests and propose changes, but repository-changing actions must be approved.

---

## Codex memory decision

The project must maintain a .codex/memory.md file.

The goal is to keep short session summaries so future agents can recover context without reading the entire conversation history.

Each relevant session should add:

- date
- agent role
- summary of what was done
- important decisions
- pending items
- validation performed

The memory file must be concise.

It must not contain secrets, tokens, passwords or sensitive environment values.

---

## Specialized agents decision

The project should use specialized agents for different responsibilities.

Initial agents:

- Software Architect
- Go Specialist
- SRE
- Performance Engineer
- Observability Analyst
- Security Engineer
- QA Engineer

The Software Architect protects boundaries and architecture.

The Go Specialist protects idiomatic Go.

The SRE protects reliability and production readiness.

The Performance Engineer protects latency, throughput, memory and concurrency.

The Observability Analyst protects the quality of logs, metrics, traces, APM analysis and technical diagnosis.

The Security Engineer protects credentials, sensitive data and secure defaults.

The QA Engineer protects test strategy and regression safety.

---

## Initial project direction

The first implementation phase should focus on a clean foundation:

1. project structure
2. configuration
3. logger
4. HTTP server
5. health check
6. core domain models
7. ports
8. first use case
9. first fake/mock provider adapter
10. tests
11. observability baseline

Avoid implementing too many real providers before the core contracts are stable.

---

## Suggested first technical milestone

The first milestone should deliver:

- Go module initialized
- project folders created
- configuration loader
- structured logger
- HTTP server using chi
- /health endpoint
- graceful shutdown
- basic OpenTelemetry setup
- basic Prometheus metrics endpoint
- first core port for LLM provider
- first core port for observability provider
- first use case for analysis orchestration
- fake provider adapter for local development
- unit tests for use case
- handler tests for HTTP layer

This creates a strong foundation before adding real providers.

---

## Non-goals for the first phase

Do not start with:

- complex plugin system
- heavy framework
- premature Kubernetes-specific logic inside the application
- multiple real providers at once
- complex async pipeline before the synchronous flow is validated
- premature optimization without measurement
- direct dependency from core to provider SDKs
- ORM-heavy persistence before query needs are clear

The first goal is to create a clean, extensible and tested core.

---

## General engineering principles

All implementation must follow:

- Clean Code
- SOLID principles
- Design Patterns when they solve a real problem
- simple package boundaries
- explicit dependencies
- small interfaces
- meaningful names
- testability
- observability
- secure defaults
- performance awareness

Avoid overengineering.

Prefer simple solutions that keep the architecture open for future providers.