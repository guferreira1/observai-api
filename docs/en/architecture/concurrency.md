# Concurrency model

ObservAI API is request-safe by design. The transport (`net/http` + chi) and all
adapters are designed so one request never reuses mutable state from another request.

## Request isolation

Each HTTP request has its own `context.Context` and request-scoped logger.
Core dependencies are either immutable or concurrency-safe:

- database pools (`pgxpool.Pool`)
- HTTP clients (`http.Client` / transport)
- cache clients (`redis.Client`)
- mutex-protected in-memory repositories/locks when used in local mode

## Shared state and safety

| Resource | Type | Why it's safe |
| --- | --- | --- |
| `ollama.Client` | HTTP client wrappers | The Go HTTP client and transport are safe for concurrent use. |
| `inmemory.AnalysisRepository` | map + `sync.RWMutex` | Guarded by lock on every access. |
| `redis.AnalysisContextCache` | Redis client | Connection-pool-safe and keys are isolated by `analysisID`. |
| `postgres.AnalysisRepository` | `pgxpool.Pool` | Pool handles concurrency and isolates DB connections. |
| `job queue` and `analysis locker` | Redis or in-memory | Either lock-per-analysis serialization or mutex map. |
| Router | configured once in `NewRouter` | Handlers are pure functions over request-scoped state. |

## What must be ordered

The only strict ordering requirement is **FIFO chat per analysis ID**.

- Different analyses can be processed in parallel.
- Messages for the same analysis are serialized through `usecase.Chat` and an
  `AnalysisLocker`.

## Queue and workers

`POST /v1/analyses` creates a job and returns quickly (`202`).

Workers consume jobs based on configured concurrency:

1. Mark job as `running`.
2. Persist phase/progress markers (`collecting`, `calling_llm`, etc.).
3. Generate analysis result.
4. Mark final status (`completed`/`failed`/`canceled`).

### Redis backend

- `legacy`: queue and worker managed through Redis lists.
- `asynq`: queue and scheduler support through Asynq.

Queue operations are safe across multiple instances because the cache lock is stored
in Redis with TTL + explicit wait policy.

## Concurrency controls for write operations

- Analysis cancellation uses job-level cancellation context.
- Chat lock TTL/Wait prevents lock leaks and starvation under high contention.
- In-memory mode still works with local mutexes and bounded worker concurrency.

## Anti-patterns intentionally avoided

- No global mutable state for user content or traces.
- No per-request creation of adapter clients.
- No unbounded goroutines for each request.
- No sharing of LLM or provider auth sessions across request scopes in mutable structures.

