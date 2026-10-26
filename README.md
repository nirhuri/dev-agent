# dev-agent

Personal CLI that takes a Jira issue through planning, implementation (via Cursor Agent), validation, commits, and a GitHub pull request.

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
```

Or build locally:

```bash
go build -o bin/dev-agent ./cmd/dev-agent
./bin/dev-agent --help
```

## First-run setup

```bash
dev-agent setup
dev-agent doctor
```

Setup writes `~/.config/dev-agent/config.yaml` (mode `0600`) and stores secrets in the **OS keyring** (macOS Keychain via `zalando/go-keyring`). Tokens never go in the config file.

## Configuration

| Source | Precedence |
|--------|------------|
| Environment (`DEV_AGENT_*`) | Highest |
| Config file | Middle |
| Built-in defaults | Lowest |

Useful commands:

```bash
dev-agent config show          # redacted view
dev-agent config set mode autonomous
dev-agent config set-secret jira_token
dev-agent config path
dev-agent doctor
```

Override paths:

- `DEV_AGENT_CONFIG_DIR` — config directory
- `DEV_AGENT_DATA_DIR` — local data (task DB)
- `--config /path/to/config.yaml` — explicit file

See [config.example.yaml](config.example.yaml) for the full schema.

## Commands (current)

| Command | Description |
|---------|-------------|
| `dev-agent` | Welcome / status when configured |
| `dev-agent setup` | First-time wizard |
| `dev-agent config` | Inspect / update settings |
| `dev-agent doctor` | Validate config and tools |
| `dev-agent issues` | List / search / select Jira issues |
| `dev-agent run KEY` | Start or attach a persisted task for an issue |
| `dev-agent status` | Show active (or `--all`) tasks |
| `dev-agent resume [ID\|KEY]` | Resume paused work / answer clarifications |

### Cursor engine

See [docs/cursor-engine.md](docs/cursor-engine.md) for verified CLI flags, automation level, and limits.

### Task persistence

Tasks are stored in SQLite at `~/.local/share/dev-agent/tasks.db` (override with `DEV_AGENT_DATA_DIR`). The driver is pure Go (`modernc.org/sqlite`) and does **not** require CGO.

States include clarification wait, failure, and cancellation so work can be inspected and resumed safely.

### Issues

```bash
dev-agent issues
dev-agent issues --project APP --status "To Do"
dev-agent issues --search auth
dev-agent issues --select APP-142
```

Interactive commands: `#` select by number, `/text` search, `f` filter, `r` refresh, `q` quit.

For automation, `DEV_AGENT_JIRA_TOKEN` can supply the API token instead of the keyring.

## Security

- Secrets live in the OS keyring, not YAML.
- `config show` and logs never print token values.
- Do not commit `.env` files or real credentials.

## Development

```bash
gofmt -w .
go vet ./...
go test ./...
```

## License

Private / personal use unless otherwise stated.
