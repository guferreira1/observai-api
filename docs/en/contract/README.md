# API Contract

ObservAI exposes a public OpenAPI contract as the runtime source for integration.

- Spec file: `internal/adapters/inbound/http/openapi.yaml`
- Served endpoint: `GET /v1/openapi.yaml`

This contract is generated to stay aligned with the Go adapter layer and is validated
by integration tests that assert route coverage and envelope behavior.

## Envelope

Every successful response uses the wrapper `WrapperDtoResponde`:

```json
{
  "data": { "...": "endpoint payload" },
  "metadata": {
    "requestId": "uuid-v4",
    "processingTimeMs": 12,
    "provider": {
      "mode": "prod",
      "observability": ["prometheus", "loki"],
      "llm": "ollama"
    },
    "warnings": [],
    "pagination": { "limit": 20, "offset": 0, "next": "/v1/analyses?limit=20&offset=20" }
  }
}
```

Error responses keep the same envelope and place machine-readable codes in
`data.code`.

### Error fields used by clients

- `code`: stable error identifier.
- `message`: short summary for UI.
- `details`: optional list with field-level validation information.

## Authentication summary

- **Browser session**
  - `POST /v1/auth/login` sets:
    - `oai_session` (HttpOnly),
    - `oai_refresh` (HttpOnly, path `/v1/auth`),
    - `oai_csrf` (JS-readable).
  - Send `X-CSRF-Token: <csrf>` on all non-GET requests.
- **API key**
  - Use `Authorization: Bearer <token>`.
- **Static + persisted keys**
  - Runtime keys can be static env-defined or persisted through admin endpoints.

## Main resources

- `POST /v1/analyses` -> async submission (`202 Accepted`) with `jobID`.
- `GET /v1/jobs/{jobID}` -> job state and progress.
- `GET /v1/analyses/{analysisID}` -> final analysis result.
- `POST /v1/analyses/{analysisID}/chat` -> scoped follow-up question.
- `GET /v1/analyses/{analysisID}/chat` -> chat history.
- `POST /v1/admin/...` -> provider, LLM, users, keys, webhooks, audit and setup resources.

## Pagination and filtering

The list endpoints accept:

- `limit` / `offset`
- `from` / `to` (RFC3339)
- `severity` (`low|medium|high|critical`)
- `service`, `signal`, `provider`, `q`, `sort`, `order`.

## Streaming

`POST /v1/analyses/{analysisID}/chat` returns standard JSON by default.

To enable SSE:

```http
Accept: text/event-stream
```

Events are emitted as:

- `token`
- `evidence_cited`
- `done`
- `error`

## Async + webhook behavior

- `POST /v1/analyses` creates job and queue payload.
- `/v1/jobs/{jobID}` returns phase/progress updates.
- Webhook configuration can notify external systems on completion/failure/cancel if enabled.

## Health and metadata endpoints

- `GET /health` and `GET /healthz`: liveness.
- `GET /readyz`: dependency readiness.
- `GET /v1/capabilities`: active provider and mode summary.
- `GET /metrics`: Prometheus exposition.

## Keeping docs and code aligned

- Runtime DTOs are in `internal/adapters/inbound/http/dto.go`.
- OpenAPI lives in `internal/adapters/inbound/http/openapi.yaml`.
- `internal/adapters/inbound/http/openapi_test.go` validates consistency.

