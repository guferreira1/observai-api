# AGENTS.md

This file defines the operating instructions for AI agents working on ObservAI API.

## Project summary

ObservAI API is an open-source, self-hosted observability analysis gateway written in Go. The API connects to observability providers, collects logs, metrics, traces and APM data, normalizes those signals and uses configurable LLM providers to generate technical diagnoses, root cause hypotheses, performance recommendations and code-level improvement suggestions.

The platform must remain provider-agnostic for both observability sources and LLM providers.

## Architecture direction

The project must follow hexagonal architecture.

The core domain must remain independent from frameworks, databases, HTTP handlers, queues, external APIs, LLM SDKs and observability provider SDKs.

Adapters must be replaceable. The business rule, normalized input model and output result must remain consistent regardless of whether the provider is Dynatrace, Datadog, Elasticsearch, Loki, Prometheus, Jaeger, OpenTelemetry, OpenAI, Anthropic, Gemini, Ollama or another supported integration.

## API contract

The contract between frontend and backend must be unique and provider-agnostic.

All successful HTTP responses must be wrapped by `WrapperDtoResponde`, containing `data` and `metadata`.

The `data` field contains the endpoint-specific payload. The `metadata` field contains response metadata such as request identifier, pagination, processing time, provider summary and warnings when applicable.

Provider-specific response fields must not leak to the frontend contract. Adapters must translate provider data into normalized domain models and API DTOs before returning a response.

## Interaction chat guardrail

The application chat must answer only questions related to an existing ObservAI analysis, its evidence, hypotheses, recommendations, affected services, logs, metrics, traces, APM data or follow-up investigation steps.

The chat must not answer unrelated general questions, coding questions, personal questions, operational requests outside the analysis scope, or prompts that try to bypass this restriction.

When the user asks for something outside the current analysis scope, the chat must refuse briefly and redirect the user to ask about the active analysis.

When evidence is missing, the chat must say that the current analysis does not contain enough evidence instead of inventing facts.

## Published LLM agents

The `.claude/` directory and `CLAUDE.md` are local working instructions and must not be treated as the published source for runtime LLM behavior.

Runtime LLM analysis, prompt translation and interaction chat instructions must live in versioned repository files outside `.claude/`, under the public `agents/` directory.

Public agent instructions must optimize token usage, reduce duplicated context, preserve relevant evidence, avoid confidential values, enforce chat scope and produce coherent, deterministic outputs.

## Engineering principles

All agents must follow Clean Code, SOLID principles, hexagonal architecture, explicit dependency direction, small interfaces owned by the consumer package, clear domain language, testability by design, operational visibility, secure configuration practices and performance awareness for high-volume logs, traces and metrics.

## Code design enforcement

Agents must follow Clean Code, SOLID and appropriate Design Patterns in every implementation.

Do not add business behavior as loose procedural code.

When a function needs more than two behavioral conditionals, extract the variation into a Strategy, Policy, Specification, Rule Object, Chain of Responsibility, Factory, Adapter or dispatcher map.

Variable and parameter names must be readable. One-letter names are forbidden except for idiomatic and obvious cases such as `t` in tests, `i` in simple loops, `ctx` for context, `id`, `tx`, `db`, `ok`, `err`, and very small HTTP handlers using `w` and `r`.

Use domain language in names. Avoid generic names like `data`, `info`, `obj`, `res`, `tmp`, `val`, `x`, `y`, `z`, `p`, `m` or `s`.

Business rules must be isolated, independently testable and named according to their domain responsibility.

## Go standards

The project must use idiomatic Go.

Rules:

- Keep packages cohesive and small.
- Prefer composition.
- Return errors explicitly.
- Wrap errors with useful context.
- Avoid global mutable state.
- Avoid unnecessary abstractions.
- Avoid framework leakage into the core.
- Use context propagation for request-scoped work.
- Use table-driven tests when it improves clarity.
- Run `gofmt` and `go test ./...` before proposing final changes.

## Comment policy

Do not add ordinary implementation comments to code.

Only GoDoc comments are allowed for exported functions, structs, interfaces, constants and packages. GoDocs must explain the purpose and responsibility of the exported element.

## Git safety rules

Agents must not perform repository write operations such as push, pull, commit, force update, rebase, merge into `main` or direct changes on `main` unless the human owner explicitly authorizes the action.

Agents may inspect files, propose diffs, create local changes and document what was changed.

## Execution safety

Agents must prefer dry-run or plan modes for commands that can mutate external state, infrastructure, databases, remotes, provider accounts or production-like resources.

## Memory policy

Agents must update `.claude/memory.md` at the end of each meaningful session with a short summary of date, agent role, work performed, decisions, pending items and commands executed when relevant.

The memory must not contain confidential values.

## Agent collaboration

Use specialized agents when the task benefits from deeper focus:

- Software Architect.
- Go Specialist.
- SRE.
- Performance Engineer.
- Observability Analyst.
- Security Engineer.
- QA Engineer.

Agents must read this file and the files under `.claude/` before making changes.

## Definition of done

A task is complete only when the change respects hexagonal architecture, keeps the core free from infrastructure dependencies, updates tests when behavior changes, includes GoDocs for exported elements, avoids confidential values, respects the git safety rules and updates `.claude/memory.md`.
