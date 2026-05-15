# Public API contract

ObservAI publishes its HTTP contract as an OpenAPI 3.1 document.

The spec lives next to the HTTP adapter so the binary can serve it without
relying on a runtime filesystem:

- Source of truth: [`internal/adapters/inbound/http/openapi.yaml`](../internal/adapters/inbound/http/openapi.yaml)
- Served at runtime: `GET /v1/openapi.yaml`
- Swagger UI: `GET /docs`
- Swagger UI alias: `GET /swagger`

## Preview and linting

```bash
# Lint the spec
npx --yes @redocly/cli@latest lint internal/adapters/inbound/http/openapi.yaml

# Preview Redoc/Swagger UI locally
npx --yes @redocly/cli@latest preview-docs internal/adapters/inbound/http/openapi.yaml
```

When the API is running locally, open `http://localhost:8080/docs` to use the
embedded Swagger UI served by the API process.

## Editing rules

- The contract is provider-agnostic. Do not add fields that mirror a specific
  observability or LLM SDK shape — adapters must normalize those into the
  documented schemas before the response reaches API DTOs.
- Every successful response is wrapped by `WrapperDtoResponde{data, metadata}`.
  Changing the wrapper shape is a breaking change and requires owner review.
- Error codes are the stable values mapped in
  [`internal/adapters/inbound/http/error_mapper.go`](../internal/adapters/inbound/http/error_mapper.go).
  Keep the `ErrorResponse.code` enum in sync with that file.
- Severity uses `low | medium | high | critical`; confidence uses
  `low | medium | high`. Both mirror `internal/core/domain`.

## Why hand-written YAML

The Go DTOs in `internal/adapters/inbound/http/dto.go` are stable and the
project favours readable hand-written YAML over codegen for now. Spec-first
adoption (oapi-codegen) can be revisited later if duplication becomes
expensive; see [`docs/architecture.md`](../docs/architecture.md) for context.
