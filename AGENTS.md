# AGENTS.md

This file defines the operating instructions for AI agents working on ObservAI API.

## Project summary

ObservAI API is an open-source, self-hosted observability analysis gateway written in Go. The API connects to observability providers, collects logs, metrics, traces and APM data, normalizes those signals and uses configurable LLM providers to generate technical diagnoses, root cause hypotheses, performance recommendations and code-level improvement suggestions.

The platform must remain provider-agnostic for both observability sources and LLM providers.

## Architecture direction

The project must follow hexagonal architecture.

The core domain must remain independent from frameworks, databases, HTTP handlers, queues, external APIs, LLM SDKs and observability provider SDKs.

Adapters must be replaceable. The business rule, normalized input model and output result must remain consistent regardless of whether the provider is Dynatrace, Datadog, Elasticsearch, Loki, Prometheus, Jaeger, OpenTelemetry, OpenAI, Anthropic, Gemini, Ollama or another supported integration.

## Engineering principles

All agents must follow Clean Code, SOLID principles, hexagonal architecture, explicit dependency direction, small interfaces owned by the consumer package, clear domain language, testability by design, operational visibility, secure configuration practices and performance awareness for high-volume logs, traces and metrics.

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

## Memory policy

Agents must update `.codex/memory.md` at the end of each meaningful session with a short summary of date, agent role, work performed, decisions, pending items and commands executed when relevant.

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

Agents must read this file and the files under `.codex/` before making changes.

## Definition of done

A task is complete only when the change respects hexagonal architecture, keeps the core free from infrastructure dependencies, updates tests when behavior changes, includes GoDocs for exported elements, avoids confidential values, respects the git safety rules and updates `.codex/memory.md`.
