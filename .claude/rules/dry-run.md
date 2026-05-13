# Dry-run execution rule

## Default execution mode

Agents must default to dry-run or plan mode for commands that can mutate external state.

This includes infrastructure changes, database migrations, remote git operations, provider API write operations, deployment commands, queue consumers, destructive filesystem operations and commands that call real external accounts.

## Allowed without dry-run

Read-only inspection, local file edits requested by the owner and local validation commands are allowed when they do not mutate external services or production-like resources.

Examples:

- `rg`
- `sed`
- `go test ./...`
- `gofmt`
- local documentation edits

## If dry-run is unavailable

If a command can mutate external state and has no dry-run or plan mode, do not execute it unless the owner explicitly authorizes that exact non-dry-run action.

Document the safer dry-run alternative in the handoff whenever possible.
