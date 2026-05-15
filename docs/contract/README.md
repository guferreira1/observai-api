# ObservAI API contract

The frontend consumes this API through three artifacts kept in sync:

1. **`/v1/openapi.yaml`** served by the running API (`internal/adapters/inbound/http/openapi.yaml` at build time). The canonical machine-readable contract.
2. **`docs/contract/zod-schemas.ts`** — TypeScript + Zod definitions of every public DTO. The frontend imports these schemas directly and validates network payloads with `safeParse` at the boundary so any drift surfaces as a typed error.
3. **`internal/adapters/inbound/http/dto.go`** — runtime source of truth on the Go side.

When changing any DTO, update all three. The integration test `TestOpenAPIDocumentCoversNewAdminAndAuthSurface` enforces that every path registered on the router is documented; the `TestContract*Envelope` tests pin the response envelope.

## Envelope

Every successful response uses the `WrapperDtoResponde` wrapper:

```json
{
  "data": { ... },
  "metadata": {
    "requestId": "uuid-v4",
    "processingTimeMs": 0,
    "provider": {
      "mode": "prod",
      "llm": "ollama",
      "observability": ["prometheus", "loki"]
    },
    "pagination": null
  }
}
```

Errors share the same envelope with `data` set to an `ErrorResponse`:

```json
{
  "data": {
    "code": "invalid_chat_question",
    "message": "request validation failed",
    "details": [
      {"field": "question", "rule": "required"}
    ]
  },
  "metadata": { "requestId": "..." }
}
```

## Authentication

Browser sessions:

- `POST /v1/auth/login` sets `oai_session` (HttpOnly), `oai_refresh` (HttpOnly, Path=/v1/auth) and `oai_csrf` (JS-readable) cookies.
- Send `X-CSRF-Token: <oai_csrf cookie value>` on every non-GET request.
- `POST /v1/auth/refresh` rotates all three cookies.

Service-to-service:

- `Authorization: Bearer <api_key>` for endpoints that don't depend on the browser session.

## Pagination + filtering

Most list endpoints accept `?limit=` and `?offset=`. Severity, signal, service and time-window filters are documented in the OpenAPI parameters for `/v1/analyses` and `/v1/admin/audit`.

## Streaming

`POST /v1/analyses/{id}/chat` returns plain JSON by default. Send `Accept: text/event-stream` to receive SSE events (`token`, `evidence_cited`, `done`, `error`).
