# API contract rules

## Provider-agnostic contract

The frontend/backend contract must be unique and must not change according to the observability provider or LLM provider used behind the scenes.

Provider-specific models, field names, SDK response shapes and error formats must be converted inside adapters before data reaches API DTOs.

## Response wrapper

All successful HTTP responses must use `WrapperDtoResponde`.

The wrapper contains:

- `data`: endpoint-specific response payload.
- `metadata`: response metadata such as request identifier, pagination, processing time, provider summary and non-fatal warnings.

The wrapper is part of the public API contract. Changing its shape requires explicit owner review.

## Determinism

DTOs must use stable field names, stable enum values and predictable ordering when order affects clients or tests.

LLM-generated analysis must be normalized into deterministic API response fields before it is returned to clients.
