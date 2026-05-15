# Roadmap

This roadmap lists capabilities intentionally out of scope for the initial feature-complete target and
architectural follow-ups.

## Planned provider support

- Additional LLM provider: Gemini (`generateContent` API shape).
- Additional observability providers that require deeper API surface mapping.

## Product hardening

- LLM cost tracking and budget alerts.
- Tenant-level isolation and RBAC-by-tenant.
- Prompt regression scoring against curated fixtures.
- Official Helm chart and Kubernetes deployment presets.
- Backup and restore playbooks for PostgreSQL/Redis in production.

## Platform improvements

- Graphical dashboards for queue depth, job backlog, and LLM token usage.
- Additional operational exports and alerts for webhook failures.
- Better cache invalidation for long-running analyses and stale context.

## Code-level constraints to keep

- Keep provider adapters replaceable and scoped to declared capabilities.
- Preserve one adapter per required capability in multi-capability providers.
- Keep changes in `factory.Dependencies` as the single shared dependency insertion point.
- Reuse middleware and auth stack; avoid parallel auth systems.

