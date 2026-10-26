# Troubleshooting

## doctor fails on config

Run `dev-agent setup` or `dev-agent config set …`. Check `dev-agent config path`.

## Jira authentication failed

1. Create an [API token](https://id.atlassian.com/manage-profile/security/api-tokens).
2. `dev-agent config set-secret jira_token`
3. Confirm `jira.base_url` and `jira.email` with `dev-agent config show` (token remains redacted).

## Cursor agent unavailable

Install the Cursor Agent CLI so `agent` is on PATH. See [cursor-engine.md](cursor-engine.md).

## Dirty tree on main/master

dev-agent will not stash or discard your changes. Commit, stash, or clean up manually, then resume.

## gh not authenticated

```bash
gh auth login
dev-agent doctor
```

Branch pushes are preserved if PR creation fails; fix auth and `dev-agent resume`.

## Resume says task cannot be resumed

Completed/cancelled tasks are terminal. Use `dev-agent status --all` to inspect. Start a new run with `dev-agent run KEY` only when no active task exists for that issue.

## SQLite / CGO

The store uses `modernc.org/sqlite` (pure Go). No CGO or system SQLite library is required.
