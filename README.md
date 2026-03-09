# Paige

Paige is a cron-based AI job orchestrator for [OpenCode](https://opencode.ai). It lets you schedule prompts on a repeating schedule, run them against OpenCode sessions, and pages you when the agent thinks a task is done — waiting for your confirmation before closing.

## Concept

```
Active job fires on schedule
  → Paige builds a context-enriched prompt
  → Sends it to OpenCode (opencode serve)
  → OpenCode reports PAIGE_STATUS: done / not_done
  → If done → job moves to Pending (awaiting your confirmation)
  → If not done → job stays Active, runs again next tick
  → You confirm in the TUI → job is Completed
```

## Job States

| State       | Meaning                                              |
|-------------|------------------------------------------------------|
| `active`    | Scheduled, fires on cron                             |
| `running`   | An OpenCode session is currently executing           |
| `pending`   | Agent reported done, awaiting human confirmation     |
| `completed` | Confirmed complete                                   |
| `cancelled` | Manually cancelled via the TUI                       |
| `paused`    | Temporarily disabled (not yet implemented)           |

## Quick Start

```bash
# Requires a running OpenCode server
opencode serve &

# Add a job
paige add \
  --name "watch PR 42" \
  --repo "github.com/myorg/myrepo" \
  --prompt "Check the status of PR #42. Is it merged or has CI failed?" \
  --schedule "*/5 * * * *"

# Open the TUI
paige tui

# Or run the daemon headlessly
paige serve
```

## Tech Stack

- **Language**: Go
- **TUI**: [Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Lip Gloss](https://github.com/charmbracelet/lipgloss)
- **Scheduler**: [gocron v2](https://github.com/go-co-op/gocron)
- **Storage**: SQLite (local) — Postgres on [Railway](https://railway.app) planned
- **AI backend**: [OpenCode](https://opencode.ai) via HTTP API

## Project Structure

```
paige/
├── cmd/paige/          # Entry point and CLI commands (cobra)
├── internal/
│   ├── daemon/         # Scheduler loop and job execution
│   ├── job/            # Domain types: Job, Run, state machine
│   ├── opencode/       # HTTP client for OpenCode API
│   ├── store/          # Store interface + SQLite implementation
│   └── tui/            # Bubble Tea TUI views
├── Makefile
└── go.mod
```

## Development

```bash
make build    # compile
make run      # build + open TUI
make test     # run tests
make tidy     # go mod tidy
```

## Roadmap

- [ ] Full TUI: job detail, run history, confirm/cancel flow
- [ ] Interactive `paige add` wizard
- [ ] Context injection (repo info, PR details, file contents)
- [ ] Structured output from OpenCode (JSON status response)
- [ ] Notification support (desktop, Slack, etc.)
- [ ] Remote mode: Paige API server on Railway, TUI as remote client
- [ ] Postgres backend for remote deployments
