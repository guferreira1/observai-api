# Provider adapter skill

Use this playbook when adding or changing an observability or LLM provider adapter.

## Steps

1. Identify the port the provider must implement.
2. Keep provider SDK usage inside the adapter package.
3. Convert provider data into internal models at the boundary.
4. Add context propagation and timeout support.
5. Sanitize provider errors before crossing API boundaries.
6. Add structured logs and metrics for provider calls.
7. Add tests for mapping and error behavior.

## Provider rule

Provider adapters may differ internally, but the core result must stay consistent.
