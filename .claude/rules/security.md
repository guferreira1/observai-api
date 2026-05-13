# Security rules

## Credentials

Never store credentials, API keys, tokens, passwords, private certificates or real secrets in the repository.

Use environment variables, secret managers or local ignored files.

## Provider access

Prefer read-only credentials for observability providers whenever possible.

Provider adapters must avoid collecting unnecessary sensitive data.

## Data handling

Logs, traces and APM events may contain personal or confidential data.

Normalize and minimize data before sending it to LLM providers.

## LLM safety

The user owns the provider, token and data flow.

LLM adapters must make the target provider explicit and must not silently forward data to another provider.

## Secure defaults

- Fail closed when configuration is incomplete.
- Do not log secrets.
- Avoid exposing internal stack traces to API consumers.
- Validate inbound payloads.
- Add timeouts to external calls.
- Keep provider errors sanitized at API boundaries.
