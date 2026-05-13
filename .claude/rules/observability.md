# Observability rules

## Goal

ObservAI API must be observable by design.

The product analyzes observability data, so its own runtime behavior must also be easy to inspect.

## Logging

- Use structured logs.
- Include correlation identifiers when available.
- Do not log credentials or sensitive payloads.
- Log provider, operation and high-level outcome.
- Avoid noisy logs inside high-volume loops.

## Metrics

Track relevant metrics for:

- Request latency.
- Request count.
- Error count.
- Provider call latency.
- Provider failures.
- LLM call latency.
- LLM failures.
- Queue or worker backlog when async processing exists.

## Tracing

Propagate context across inbound requests, use cases and outbound adapters.

External calls should be traceable by provider, operation and duration.

## Health checks

Expose simple readiness and liveness checks.

Readiness should reflect critical dependencies when applicable.
