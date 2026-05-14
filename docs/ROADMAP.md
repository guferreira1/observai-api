# Roadmap

This document lists the work explicitly **out of scope** of the current
"feature-complete v1" wave. Items live here either because they would
multiply complexity without proportional value, or because they require
product-level decisions that have not been made yet.

The architecture (hexagonal core + dispatcher-based outbound factory +
ports) is intentionally shaped so each item below can be added without
touching existing code paths.

## Provider expansion

| Provider     | Capability           | Notes                                                                        |
| ------------ | -------------------- | ---------------------------------------------------------------------------- |
| Gemini       | LLM                  | Distinct payload shape (`generateContent`). Reuses the LLM dispatcher slot.  |
| Datadog      | logs + metrics + traces + APM | Multi-API surface, each endpoint distinct; needs careful pagination. |
| Dynatrace    | logs + metrics + traces + APM | Auth uses `Api-Token`; APM relies on the Smartscape model.           |
| New Relic    | logs + metrics + traces + APM | NRQL is its own query language; pricing model gates traces.          |
| Elastic APM  | APM                  | Reuses the Elasticsearch client with APM-specific indices.                   |

For each commercial provider, weigh demand before investing — a single
adapter is multiple days of work and pulls a significant maintenance tail.

## Cost tracking for LLM calls

BYO LLM means the operator pays the provider bill. To surface usage:

- Extend `ports.AnalysisGenerator` / `ports.ChatResponder` so adapters
  can return token-usage metadata alongside the result (`prompt`,
  `completion`, `total`, `cost_estimate_usd`).
- Persist usage rows alongside `analyses` and `analysis_chat_messages`
  via a new `llm_usage` table; expose `/v1/analyses/{id}/usage` and a
  global aggregate at `/v1/admin/usage`.
- Add a UI panel in `observai-web` that renders cost per analysis.

## Multi-tenancy

The current domain has no `tenant_id` field. Adding multi-tenancy is a
domain-wide refactor:

- Every persisted row needs a `tenant_id` column with a non-nullable FK.
- Row-level security policies in PostgreSQL gate per-tenant access.
- The auth middleware sets the tenant from the API key; the audit log
  records the tenant alongside the actor.
- All admin endpoints become tenant-scoped except for a super-admin
  surface that lists tenants.

Recommended only when SaaS deployment is on the table.

## Prompt evaluation framework

Analysis quality is the product. Building confidence in prompt changes
requires:

- A curated dataset of (request, evidence, expected diagnosis).
- A scorer that compares the LLM output against expected severities,
  affected services and root-cause categories.
- CI step that fails when a prompt change drops the aggregate score
  below the current main-branch baseline.

This is its own project; not blocking for v1.

## Operational packaging

- **Helm chart / Kubernetes manifests** under `deploy/k8s` referencing
  the published image, the embedded Postgres migrations and a sensible
  `values.yaml`.
- **Grafana dashboards** committed as JSON under `deploy/grafana/` so
  operators can import them directly. The metrics already exist; only
  the dashboards are missing.
- **Backup playbook** under `docs/operations/` describing `pg_dump`
  cadence, Redis durability options and disaster-recovery drills.

## Notes for future agents

- Adapters that consume multiple capabilities (Datadog, Dynatrace, New
  Relic) should still be split into discrete files per capability so
  the dispatcher can wire only what the operator declared.
- The `factory.Dependencies` struct is the single insertion point for
  shared collaborators; do not bypass it by passing the logger or
  observer through globals.
- Multi-tenancy must enter through the same middleware that resolves
  API keys today; do not introduce a parallel auth layer.
