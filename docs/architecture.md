# Architecture

Modular monolith in Go. Packages stay small and introduce interfaces only at real seams (engine, store, PR provider).

```text
cmd/dev-agent          main
internal/cli           Cobra commands + interactive UX
internal/config        schema, load/save, validation, redaction
internal/secrets       OS keyring
internal/jira          Jira Cloud REST client
internal/workflow      task states + service
internal/store         SQLite persistence (modernc.org/sqlite, no CGO)
internal/engine        coding-engine interface + fake
internal/engine/cursor Cursor `agent` CLI adapter
internal/repository    Git + validation
internal/pr            GitHub via `gh`
internal/doctor        environment checks
internal/e2e           end-to-end tests with fakes
```

## Task state machine

```text
IDLE → ISSUE_SELECTED → ANALYZING → WAITING_FOR_CLARIFICATION?
     → PLAN_READY → IMPLEMENTING → VALIDATING ⇄ REPAIRING
     → READY_FOR_PR → PR_CREATED → COMPLETED

Also: BLOCKED, FAILED, CANCELLED
```

## Adding a coding engine

1. Implement `engine.Engine` in `internal/engine/<name>`.
2. Wire construction in `internal/cli/engine.go`.
3. Set `engine.provider` in config.
4. Keep provider-specific types inside the adapter package.

## Secrets

Tokens live in the OS keyring (`jira_token`, `cursor_api_key`). Config YAML never stores them. `DEV_AGENT_JIRA_TOKEN` / `CURSOR_API_KEY` work for CI/automation.
