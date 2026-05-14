# Codex memory

This file stores concise session notes for ObservAI API.

## Entry format

```md
### YYYY-MM-DD - Agent role

- Summary: short description of what was done.
- Decisions: relevant technical decisions.
- Pending: remaining items or follow-up work.
- Validation: commands or checks executed.
```

## Entries

### 2026-05-13 - Onda 1 fake removal + real ID/feedback wiring

- Summary: removed the unsafe `internal/adapters/outbound/fake` package from the production path. Created `internal/adapters/outbound/uuid` with a `UUIDv7` `IDGenerator` (now wired in `cmd/observai-api/main.go`); created `internal/adapters/outbound/null` exposing `SignalCollector`, `AnalysisGenerator`, `ChatResponder` and `TraceProvider` that return `domain.ErrProviderNotConfigured` instead of synthetic data; added `domain.ErrProviderNotConfigured` sentinel; renamed legitimate in-memory fallbacks (analysis/job/chat-feedback repositories, context cache, queue/worker/enqueuer, locker) to `internal/adapters/outbound/inmemory`; added migration `000006_create_chat_feedback` + `query/chat_feedback.sql`, regenerated sqlc, and added `postgres.ChatFeedbackRepository` so the use case no longer relies on the in-memory feedback fallback in prod; moved deterministic test doubles to `internal/adapters/outbound/testfakes` (signal collector, analysis generator, chat responder, id generator and a new trace provider that returns a four-span checkout trace shaped for `CriticalPathSpanIDs[0] == "span-root"`).
- Decisions: chose UUIDv7 over ULID because the existing `analyses.id` column is `TEXT` (no migration needed), UUIDv7 is RFC 9562 and `google/uuid` v1.6 was already a transitive dep; chose to fail-closed in local/dev when a provider URL is not configured (null adapters surface `ErrProviderNotConfigured`) instead of generating synthetic evidence; kept the deterministic test doubles in a clearly named `testfakes` package so production code cannot accidentally re-introduce them via import, and updated `testfakes.ChatResponder` to return evidence IDs (`ev_*`) instead of names to match the new chat contract expected by `internal/core/usecase/chat_test.go`.
- Pending: Onda 2 (factory + dispatcher map for multi-provider config), Onda 3 (real Loki/Jaeger/OpenAI adapters), Onda 4 (redaction policy, credential store, auth middleware); run `migrate up` against the local Postgres to apply migration `000006`; add `integration` build-tagged Postgres test for `ChatFeedbackRepository` if not already covered.
- Validation: ran `/home/gustavo/go/bin/sqlc generate`; ran `gofmt -w cmd internal`; ran `GOCACHE=/tmp/observai-go-build-cache GOMODCACHE=/tmp/observai-go-mod-cache go mod tidy`; ran `GOCACHE=/tmp/observai-go-build-cache GOMODCACHE=/tmp/observai-go-mod-cache go vet ./...` (clean); ran `GOCACHE=/tmp/observai-go-build-cache GOMODCACHE=/tmp/observai-go-mod-cache go build ./...` (clean); ran `GOCACHE=/tmp/observai-go-build-cache GOMODCACHE=/tmp/observai-go-mod-cache go test ./...` (all packages OK).

### 2026-05-13 - Phase 6 public contract + DX

- Summary: published the API contract as a hand-written OpenAPI 3.1 document embedded in the HTTP adapter and served at `GET /v1/openapi.yaml`; enriched the runtime LLM prompts in `agents/observability-analysis-agent.md` and `agents/interaction-chat-agent.md` with strict JSON schemas, refusal envelope, few-shot examples and prompt-injection countermeasures; added a build-tagged end-to-end integration test (`internal/adapters/inbound/http/integration_test.go`, tag `integration`) that spins up a real PostgreSQL container via testcontainers-go, runs `golang-migrate` programmatically, and drives `POST /v1/analyses → GET /v1/analyses/:id → GET /v1/analyses → POST /v1/analyses/:id/chat → GET /v1/analyses/:id/chat` plus the out-of-scope rejection path; created `docs/architecture.md` and `api/README.md`.
- Decisions: spec lives at `internal/adapters/inbound/http/openapi.yaml` (embed.FS does not allow `..` paths) and `api/README.md` documents that the embed file is the source of truth; kept the same `fake.*` adapters in the integration test so chat history exercises the Postgres `analysis_chat_messages` table while the LLM responder stays deterministic; chose `tcpostgres.WithSQLDriver` is unnecessary because we open the repository with pgx directly via `postgres.NewAnalysisRepository`; build-tag gates the testcontainer test so `go test ./...` stays Docker-free.
- Pending: optional Phase 6 extras (circuit breaker on top of retry, Asynq jobs) and a real Redis/Postgres-backed integration test for the chat context cache.
- Validation: ran `gofmt -w cmd internal`; ran `GOCACHE=/tmp/observai-go-build-cache GOMODCACHE=/tmp/observai-go-mod-cache go vet ./...`; ran `GOCACHE=/tmp/observai-go-build-cache GOMODCACHE=/tmp/observai-go-mod-cache go build ./...`; ran `GOCACHE=/tmp/observai-go-build-cache GOMODCACHE=/tmp/observai-go-mod-cache go test ./...`; ran `GOCACHE=/tmp/observai-go-build-cache GOMODCACHE=/tmp/observai-go-mod-cache go test -tags=integration -count=1 -timeout=5m ./internal/adapters/inbound/http/...` (Docker available, 6.4s, all subtests passing including the out-of-scope rejection path).

### 2026-05-12 - Robustness phase (retry + graceful shutdown)

- Summary: added `internal/platform/retry` with bounded exponential backoff and full jitter, integrated it into the Ollama and Prometheus clients replacing the manual retry loop, classified Prometheus transient errors (network + 5xx) and locked behavior with regression tests; wired `cfg.HTTPShutdownTimeout` into `main.go` so the HTTP server drains in-flight requests with the configured grace period and falls back to `srv.Close()` if shutdown fails.
- Decisions: kept a local `transientError` type per adapter to preserve type-safe classification without leaking infra concerns across packages; defaulted `retry.Default()` to 3 attempts, 100ms base, 2s cap; left tracer shutdown sharing the same drain context; on shutdown timeout the server force-closes rather than os.Exit so the rest of the orderly teardown still runs.
- Pending: Phase 6 work (OpenAPI generation, runtime agent prompt files under `agents/`, Postgres testcontainers integration test, `docs/architecture.md`); optional circuit breaker on top of retry is still open.
- Validation: ran `gofmt -w internal`; ran `GOCACHE=/tmp/observai-go-build-cache go vet ./...`; ran `GOCACHE=/tmp/observai-go-build-cache go build ./...`; ran `GOCACHE=/tmp/observai-go-build-cache go test ./...`.

### 2026-05-12 - Onda 1 configuration and analysis listing

- Summary: added YAML-based configuration loading through `OBSERVAI_CONFIG_FILE` with environment overrides, documented `config.example.yaml` and `.env.example`, added paginated `GET /v1/analyses` with service/severity filters, wired list support through the analysis use case and fake/PostgreSQL repositories, and strengthened CI with Postgres service, sqlc compile/diff and migration checks.
- Decisions: kept YAML as the flexible baseline while preserving environment overrides for deploy/runtime secrets; placed the example YAML under `config/config.example.yaml`; used `ListAnalyses` on the analysis repository port to avoid colliding with chat history `List`; used local `sqlc compile` and `sqlc diff` instead of `sqlc verify` because `sqlc verify --no-remote` still attempted remote access.
- Pending: define the final HTTP error wrapper contract, add OpenAPI, add stronger request body limits/timeouts and add severity/confidence database constraints in the next wave.
- Validation: ran `sqlc generate`; ran `sqlc compile`; ran `sqlc diff`; ran `gofmt -w internal/platform/config internal/core internal/adapters cmd`; ran `GOCACHE=/tmp/observai-go-build-cache go test ./...`; ran `GOCACHE=/tmp/observai-go-build-cache go vet ./...`; ran `GOCACHE=/tmp/observai-go-build-cache go test ./... -race -coverprofile=coverage.out`; ran PostgreSQL integration tests with `OBSERVAI_TEST_DATABASE_DSN`; ran Redis integration tests with `OBSERVAI_TEST_REDIS_URL`; checked local migration version with `migrate`.

### 2026-05-12 - Roadmap planning

- Summary: produced an attack plan for the pending roadmap, grouping related items into closed implementation waves and prioritizing contract/domain hardening before real provider and LLM adapters.
- Decisions: first wave should close CI, HTTP error contract, config/docs baseline, analysis list pagination and domain error mapping; Prometheus and Ollama should follow only after normalized contracts and observability/security baselines are in place.
- Pending: implement the selected waves in dedicated branches and validate each with `gofmt`, `go test ./...`, CI checks and adapter-specific integration tests where applicable.
- Validation: read `.codex/README.md`, relevant `.codex/rules/*` files and recent `.codex/memory.md`; no application tests were run because this was planning-only.

### 2026-05-12 - GET analysis endpoint

- Summary: added the `GET /v1/analyses/{analysisID}` HTTP endpoint backed by the analysis use case and existing `AnalysisRepository.Find` port; reused the provider-agnostic analysis DTO and response wrapper.
- Decisions: kept retrieval in the core use case instead of reading repositories directly from the HTTP adapter; preserved domain error mapping for missing analyses.
- Pending: implement paginated `GET /v1/analyses` and decide the final error wrapper contract before publishing OpenAPI.
- Validation: ran `gofmt -w internal/core/usecase/analysis.go internal/core/usecase/analysis_test.go internal/adapters/inbound/http/router.go internal/adapters/inbound/http/router_test.go`; ran `GOCACHE=/tmp/observai-go-build-cache go test ./...`.

### 2026-05-12 - Pattern refactor pass

- Summary: reviewed the existing Go code for conditional business rules and applied targeted patterns: chat scope Policy, Null Object defaults for optional cache/history, HTTP domain error mapper rules, PostgreSQL Data Mapper functions and composition-root adapter factories.
- Decisions: kept chat scope enforcement inside the core use case layer; kept HTTP error translation in the inbound adapter; kept PostgreSQL domain/sqlc conversion inside the outbound adapter; kept provider/cache selection in `cmd/observai-api` so business rules do not depend on infrastructure choices.
- Pending: future provider integrations should follow the same pattern boundaries and avoid adding provider-specific decisions inside core use cases.
- Validation: ran `gofmt -w cmd internal`; ran `GOCACHE=/tmp/observai-go-build-cache go test ./...`; noted that plain `go test ./...` previously failed because the sandbox cannot write to `/home/gustavo/.cache/go-build`.

### 2026-05-12 - Pattern strategy rule

- Summary: added a local agent rule requiring Strategy, Policy, Specification, Rule Object or dispatcher extraction when more than two behavioral `if` statements appear in the same block, function or method.
- Decisions: documented the rule in `AGENTS.md` for immediate agent guidance and in `.codex/rules/pattern-strategy.md` as a dedicated maintainability rule; kept guard clauses for errors, nil checks, context cancellation and simple validation explicitly allowed.
- Pending: apply this rule during future implementation and reviews, especially around provider selection, normalization, LLM behavior, chat scope, response shaping and persistence decisions.
- Validation: documentation-only change; read `AGENTS.md`, `.codex/README.md`, `.codex/rules/*`, `.codex/agents/*`, `.codex/skills/*`, `.codex/hooks/*`, `.codex/tasks/*` and `.codex/memory.md`; checked branch with `git branch --show-current`; checked worktree with `git status --short`.

### 2026-05-12 - PostgreSQL persistence foundation

- Summary: added PostgreSQL persistence foundation with pgx repository adapter for analyses, initial golang-migrate migration files for analyses and chat messages, sqlc configuration and query files, optional database DSN configuration and runtime wiring that uses PostgreSQL when `OBSERVAI_DATABASE_DSN` is set.
- Decisions: kept the core dependent only on `ports.AnalysisRepository`; stored nested normalized analysis fields as `jsonb` inside the PostgreSQL adapter; kept the existing fake repository as the default local fallback when no database DSN is configured.
- Pending: run real migrations against the local PostgreSQL instance and add chat persistence through a dedicated core port when chat history becomes required behavior.
- Validation: ran `gofmt -w cmd internal`; ran `GOCACHE=/tmp/observai-go-build-cache go mod tidy`; ran `GOCACHE=/tmp/observai-go-build-cache go test ./...`; checked that `sqlc` and `migrate` are not installed.

### 2026-05-12 - SQL tool installation and generation

- Summary: installed `sqlc` and `migrate` into `/home/gustavo/go/bin`, regenerated sqlc files for PostgreSQL queries and updated the PostgreSQL analysis repository to use generated sqlc query methods instead of handwritten SQL constants.
- Decisions: compiled `migrate` with the `postgres` build tag so PostgreSQL URLs are supported; did not apply database migrations because that mutates local database state and was not explicitly requested.
- Pending: apply migrations against local PostgreSQL when authorized; consider adding integration tests with a disposable PostgreSQL container.
- Validation: ran `go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`; ran `go install -tags postgres github.com/golang-migrate/migrate/v4/cmd/migrate@latest`; ran `sqlc version`; ran `migrate -version`; ran `sqlc generate`; ran `gofmt -w cmd internal`; ran `GOCACHE=/tmp/observai-go-build-cache go test ./...`.

### 2026-05-12 - Local PostgreSQL migration applied

- Summary: applied the initial PostgreSQL migration to the local Docker Compose database, creating `analyses`, `analysis_chat_messages` and `schema_migrations`.
- Decisions: used the local development DSN `postgres://observai:observai@localhost:5432/observai?sslmode=disable`; kept migration execution limited to the local Compose database.
- Pending: add repository integration tests against a disposable PostgreSQL instance and decide when chat history should become a core port/use case concern.
- Validation: ran `docker compose ps postgres`; ran `migrate -path migrations -database 'postgres://observai:observai@localhost:5432/observai?sslmode=disable' up`; ran `migrate -path migrations -database 'postgres://observai:observai@localhost:5432/observai?sslmode=disable' version`; ran `docker compose exec -T postgres psql -U observai -d observai -c "\\dt"`; ran `GOCACHE=/tmp/observai-go-build-cache go test ./...`.

### 2026-05-12 - PostgreSQL repository integration tests

- Summary: added PostgreSQL integration tests for `AnalysisRepository` save/find round-trip and not-found behavior, gated by `OBSERVAI_TEST_DATABASE_DSN` so default test runs do not require a database.
- Decisions: tests clean up their own analysis row by ID; normal `go test ./...` keeps the integration tests skipped unless the database DSN is explicitly provided.
- Pending: decide the first Redis-backed behavior before adding a Redis core port to avoid unused abstractions.
- Validation: ran `gofmt -w internal/adapters/outbound/postgres/analysis_repository_integration_test.go`; ran `GOCACHE=/tmp/observai-go-build-cache go test ./...`; ran `OBSERVAI_TEST_DATABASE_DSN='postgres://observai:observai@localhost:5432/observai?sslmode=disable' GOCACHE=/tmp/observai-go-build-cache go test ./internal/adapters/outbound/postgres -run Integration -count=1 -v`.

### 2026-05-12 - Redis analysis context cache and durable chat history

- Summary: added `AnalysisContext`, `AnalysisContextCache` and `ChatHistoryRepository` ports; implemented Redis-backed analysis context caching with a 6h default TTL; implemented durable chat history in PostgreSQL using the existing `analysis_chat_messages` table; added `GET /v1/analyses/{analysisID}/chat` to retrieve persisted chat history.
- Decisions: PostgreSQL remains the source of truth for analyses and durable chat history; Redis is an optimization for compact analysis context and cache failures do not fail analysis/chat when PostgreSQL is available; chat history persistence errors fail chat requests to avoid reporting unsaved history as successful.
- Pending: add real LLM/Ollama adapter so persisted chat answers come from runtime prompts instead of the deterministic fake responder; consider adding observability metrics for Redis cache hit/miss and chat history persistence failures.
- Validation: ran `go get github.com/redis/go-redis/v9`; ran `gofmt -w cmd internal`; ran `GOCACHE=/tmp/observai-go-build-cache go mod tidy`; ran `GOCACHE=/tmp/observai-go-build-cache go test ./...`; ran `OBSERVAI_TEST_DATABASE_DSN='postgres://observai:observai@localhost:5432/observai?sslmode=disable' GOCACHE=/tmp/observai-go-build-cache go test ./internal/adapters/outbound/postgres -run Integration -count=1 -v`; ran `OBSERVAI_TEST_REDIS_URL='redis://localhost:6379/0' GOCACHE=/tmp/observai-go-build-cache go test ./internal/adapters/outbound/redis -run Integration -count=1 -v`.

### 2026-05-12 - Development compose adjustment

- Summary: changed the Docker Compose stack back to infrastructure-only for local development, removed the API service from Compose, removed the temporary Dockerfile and `.dockerignore`, and pointed Prometheus to the locally running API through `host.docker.internal:8080`.
- Decisions: during development the API runs locally with Go while Compose provides PostgreSQL, Redis, Elasticsearch, Logstash, Kibana, Prometheus, Grafana and Ollama.
- Pending: recreate a separate production/self-hosted Compose file and Dockerfile when publication becomes the active goal.
- Validation: ran `docker compose -f docker-compose.yml config --quiet`; ran `GOCACHE=/tmp/observai-go-build-cache go test ./...`.

### 2026-05-12 - Docker stack validation

- Summary: validated the Docker Compose stack, fixed the missing `observai-api` service, added a production-style multi-stage Dockerfile, added `.dockerignore`, removed unused Go cache volumes from Compose and added service health dependencies for API, PostgreSQL, Redis and Elasticsearch consumers.
- Decisions: kept the stack aligned with project decisions using PostgreSQL, Redis, Elasticsearch, Logstash, Kibana, Prometheus, Grafana, Ollama and the Go API; added CA certificates to the API runtime image for future HTTPS provider calls.
- Pending: consider adding real readiness checks for Prometheus, Grafana, Kibana and Ollama if startup ordering becomes flaky; externalize local default passwords before non-local deployment.
- Validation: ran `docker compose -f docker-compose.yml config`; ran `docker compose -f docker-compose.yml build observai-api`; ran `GOCACHE=/tmp/observai-go-build-cache go test ./...`.

### 2026-05-12 - Local compose stack

- Summary: created `docker-compose.yml` for a local ObservAI stack with the API, Postgres, Redis, Prometheus, Grafana, Elasticsearch, Logstash, Kibana and Ollama support.
- Decisions: kept the stack local-only with no `restart: always`; wired Prometheus to the API `/metrics` endpoint; provisioned Grafana datasources for Prometheus and Elasticsearch; left the app running through `go run` inside a Go container for easier local iteration.
- Pending: the repository still does not expose production adapters for Postgres, Redis or ELK, so those services are infrastructure-ready but not yet used by the current fake adapters.
- Validation: ran `docker compose -f docker-compose.yml config`.

### 2026-05-12 - Initial Go slice

- Summary: removed `AGENTS.md` and `.codex/` from Git tracking, created the initial Go module, added the first hexagonal vertical slice, implemented `WrapperDtoResponde`, fake observability and LLM adapters, scoped analysis chat guardrail, chi HTTP router, cleanenv config, Prometheus metrics, OpenTelemetry HTTP wrapping and graceful shutdown.
- Decisions: followed `.codex/rules/project-decisions.md`; kept framework and telemetry dependencies outside the core; enforced chat scope in the core use case before invoking any responder.
- Pending: replace fake adapters with the first real read-only observability adapter and configurable LLM adapter; review whether the public Go type name should remain `WrapperDtoResponde`.
- Validation: ran `gofmt -w cmd internal`; ran `go mod tidy`; ran `GOCACHE=/tmp/observai-go-build-cache go test ./...`.

### 2026-05-12 - Planning and runtime agent setup

- Summary: added API contract rules for `WrapperDtoResponde`, dry-run execution guidance, interaction chat guardrails and public runtime LLM agent instructions.
- Decisions: runtime LLM instructions should live under `agents/` because `.codex/` and `AGENTS.md` are local working instructions; the interaction chat must only answer questions about an active analysis.
- Pending: owner review of the exact exported Go name `WrapperDtoResponde` before implementation, then start the thin vertical slice with fake adapters and deterministic tests.
- Validation: documentation-only change; inspected README, AGENTS, `.codex` rules, git status and tracked files.

### 2026-05-12 - Documentation setup

- Summary: created the initial Codex workspace documentation for project rules, specialized agents, hooks, skills and task handoff.
- Decisions: documented hexagonal architecture as the default style and provider adapters as replaceable boundary implementations.
- Pending: owner review of the created structure and future expansion as implementation begins.
- Validation: documentation-only change; no application tests were required.
