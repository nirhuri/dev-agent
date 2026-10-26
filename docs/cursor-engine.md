# Cursor coding-engine adapter

## Verified interface (local environment)

Installed CLI: `agent` (Cursor Agent, version observed via `agent --version`).

Relevant flags confirmed from `agent --help`:

| Flag | Role |
|------|------|
| `--print` / `-p` | Non-interactive output for scripts |
| `--output-format text\|json\|stream-json` | Machine-readable results with `--print` |
| `--workspace <path>` | Target repository/workspace |
| `--trust` | Trust workspace without prompting |
| `--force` / `--yolo` | Auto-approve tool calls (autonomous mode only) |
| `--resume [chatId]` | Continue a prior session |
| `--model <model>` | Model selection |
| `CURSOR_API_KEY` / `--api-key` | Auth for unattended use |

Automation level reported by `dev-agent doctor`: **unattended** when `agent` is on PATH; otherwise **unavailable**.

## How dev-agent uses it

`internal/engine/cursor` builds a task prompt from the Jira issue, plan, and clarifications, then runs:

```bash
agent --print --output-format text --trust --workspace <repo> [--force] [--model ...] "<prompt>"
```

- Interactive mode omits `--force` so the agent may still prompt for sensitive tools depending on CLI policy.
- Autonomous mode passes `--force`.
- API key is read from the OS keyring or `CURSOR_API_KEY` and passed via environment (never logged).

## Limitations

- The adapter does not parse Cursor GUI state or drive the IDE with UI automation.
- Success means the `agent` process exited 0; callers must still verify git diffs / tests.
- Session IDs for `--resume` depend on CLI output; until parsed reliably, continue uses a fresh prompt with prior context embedded.
- Hybrid fallback: if `agent` is missing, doctor reports unavailable and the workflow stops before claiming code was changed.

## Adding another engine

Implement `engine.Engine` in a new package under `internal/engine/<name>` and select it via `engine.provider` in config. Keep Cursor-specific types inside the cursor package.
