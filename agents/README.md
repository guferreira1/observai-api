# ObservAI public agents

This directory contains versioned instructions for runtime LLM behavior.

These files are different from local assistant rules under `.codex/`. The application can use these instructions to build compact prompts for analysis, prompt translation and interaction chat without depending on uncommitted local workspace files.

## Goals

- Keep ObservAI provider-agnostic.
- Minimize tokens sent to LLM providers.
- Preserve evidence required for accurate diagnosis.
- Avoid confidential values and unnecessary raw payloads.
- Restrict the interaction chat to analysis-related questions.
- Produce deterministic, structured and actionable outputs.

## Agent files

- `observability-analysis-agent.md`: analyzes normalized observability evidence.
- `prompt-translator-agent.md`: converts user intent into compact internal analysis requests.
- `interaction-chat-agent.md`: answers only follow-up questions about an active analysis.

## Usage rules

Runtime prompts must send normalized signals, not provider SDK payloads.

Inputs should be summarized before reaching the LLM when raw payloads are large, duplicated or sensitive.

The LLM output must be validated and mapped into the public API response contract before returning it to clients.
