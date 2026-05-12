# Go rules

## Style

Use idiomatic Go.

Prefer simple code, cohesive packages and explicit dependencies.

## Package design

- Keep package names short and meaningful.
- Avoid package names such as `common`, `utils` or `helpers` unless there is a strong reason.
- Keep interfaces close to the consumer.
- Avoid large interfaces.
- Prefer constructor functions for dependencies.
- Avoid hidden side effects in package initialization.

## Errors

- Return errors explicitly.
- Add context when returning errors across boundaries.
- Keep domain errors understandable.
- Do not expose provider implementation details from core use cases.

## Context

Use `context.Context` for request-scoped operations, external calls, database operations and long-running processing.

Do not store context inside structs.

## Tests

- Use table-driven tests when useful.
- Test behavior, not private implementation details.
- Add unit tests for use cases and domain behavior.
- Add adapter tests where provider conversion logic is relevant.
- Keep tests readable and deterministic.

## Documentation

Exported elements require GoDocs.

Do not add ordinary implementation comments. Prefer clearer code and good names.
