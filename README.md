# dev-agent

Personal CLI that takes a Jira issue through planning, implementation (via Cursor Agent), validation, commits, and a GitHub pull request — without auto-merging.

## Prerequisites

- Go 1.22+ (developed with Go 1.27)
- Git
- [GitHub CLI](https://cli.github.com/) (`gh`) for PR creation
- [Cursor Agent CLI](https://cursor.com/docs) (`agent`) for coding-engine automation
- Jira Cloud account with an [API token](https://id.atlassian.com/manage-profile/security/api-tokens)

## Install

```bash
git clone https://github.com/nirhuri/dev-agent.git
cd dev-agent
go install ./cmd/dev-agent
# or: make build && ./bin/dev-agent --help
```

## First-run setup

```bash
dev-agent setup
dev-agent doctor
```

Setup writes `~/.config/dev-agent/config.yaml` (mode `0600`) and stores secrets in the **OS keyring**. Tokens never go in the config file.

## Happy path

```bash
dev-agent issues              # list / select
dev-agent run APP-142         # create persisted task + plan
dev-agent resume              # implement → validate → commit → PR
dev-agent status              # inspect active work
```

## Commands

| Command | Description |
|---------|-------------|
| `dev-agent` | Welcome / status when configured |
| `dev-agent setup` | First-time wizard |
| `dev-agent config` | Inspect / update settings |
| `dev-agent doctor` | Validate config and tools |
| `dev-agent issues` | List / search / select Jira issues |
| `dev-agent run KEY` | Start or attach a persisted task |
| `dev-agent status` | Show active (or `--all`) tasks |
| `dev-agent resume [ID\|KEY]` | Resume work / answer clarifications / continue pipeline |

### Issues

```bash
dev-agent issues
dev-agent issues --project APP --status "To Do"
dev-agent issues --select APP-142
```

Interactive: `#` select, `/text` search, `f` filter, `r` refresh, `q` quit.

## Configuration

| Source | Precedence |
|--------|------------|
| Environment (`DEV_AGENT_*`) | Highest |
| Config file | Middle |
| Built-in defaults | Lowest |

```bash
dev-agent config show
dev-agent config set mode autonomous
dev-agent config set-secret jira_token
dev-agent config path
```

Overrides: `DEV_AGENT_CONFIG_DIR`, `DEV_AGENT_DATA_DIR`, `--config`, `DEV_AGENT_JIRA_TOKEN`, `CURSOR_API_KEY`.

See [config.example.yaml](config.example.yaml).

## Safety

- Secrets in OS keyring only; redacted in `config show`
- Never commits on protected branches (`main`/`master`)
- Task branches: `agent/<KEY>-slug`
- Explicit staging only (never `git add .`)
- Refuses likely secret files
- Bounded repair attempts
- Creates PRs via `gh` but **never merges**
- Treats Jira/file content as untrusted input

## Persistence

SQLite at `~/.local/share/dev-agent/tasks.db` via pure-Go `modernc.org/sqlite` (**no CGO**).

## Docs

- [Architecture](docs/architecture.md)
- [Cursor engine](docs/cursor-engine.md)
- [Troubleshooting](docs/troubleshooting.md)

## Development

```bash
make fmt vet test
```

## Known MVP limits

- Jira Cloud read-only (no status updates/comments unless you extend later)
- GitHub PRs only (`gh`)
- Numbered-list CLI (no full-screen TUI)
- Engine success ≠ verified correct change; validation + your review still required
