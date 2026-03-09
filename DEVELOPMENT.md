# Development

## Current State

The v1 scaffold is complete and `go build ./...` passes cleanly. The following is built and functional:

- **CLI** (`cmd/paige/main.go`): Cobra commands — `serve`, `tui`, `add`, `list`, `close`
- **Domain types** (`internal/job`): `Job`, `Run`, `State`, `RunStatus` enums, constructors
- **Store interface + SQLite backend** (`internal/store`): Full CRUD for jobs and runs, schema migration on open
- **OpenCode HTTP client** (`internal/opencode`): Health check, create/delete session, send prompt, extract text
- **Daemon** (`internal/daemon`): gocron v2 scheduler, job execution loop, `PAIGE_STATUS` response parsing, state machine transitions
- **TUI root model** (`internal/tui/tui.go`): Bubble Tea program wiring, view routing
- **Job list screen** (`internal/tui/joblist.go`): Async load, state icons, `r` to refresh
- **Job detail stub** (`internal/tui/jobdetail.go`): Present but non-functional (no-op Init/Update/View)

---

## Known Gaps in the v1 Scaffold

These issues exist in the current code and should be addressed before the v1 milestone is considered done:

- **`ConfirmJob` semantics are wrong**: the function is named `ConfirmJob` but immediately transitions the job to `StateClosed`. The intent is that confirming a pending job should close it — so either the name needs fixing or the state machine needs an extra step, depending on whether a re-active flow is ever wanted.
- **`activeFilter` is declared but never wired**: `JobListModel` has an `activeFilter` field that is never set or read. Either wire it to the state filter tabs or remove it.
- **`stateLabel()` helper is defined but never called**: dead code in the TUI package.
- **`jobDetail` on root `Model` is never initialized**: navigating to job detail will panic.
- **`~/.paige/` is not auto-created on startup**: if the directory doesn't exist, the SQLite store will fail to open. The CLI should create the directory before opening the store.

---

## Milestone 0 — Fix the Scaffold

Resolve all known gaps so the tool is runnable end-to-end without panics or silent failures.

- Fix `ConfirmJob` naming/semantics
- Wire or remove `activeFilter` in `JobListModel`
- Remove `stateLabel()` dead code
- Initialize `jobDetail` in root `Model`
- Auto-create `~/.paige/` directory on startup in the CLI

---

## Milestone 1 — Functional TUI

A complete, navigable TUI that covers the core user workflow.

- Job detail view: show job metadata and full run history
- Confirm / close flow: key binding to confirm a pending job from the detail view
- Navigation: list → detail → back, keyboard-driven
- State filter tabs: filter job list by state (active, pending, closed, paused)

---

## Milestone 2 — Better Job Creation

Make creating and managing jobs more ergonomic.

- Interactive `paige add` wizard using a Bubble Tea form (huh or manual)
- Cron expression validation with a human-readable preview
- `paige pause` / `paige resume` subcommands
- Ability to edit an existing job's prompt or schedule

---

## Milestone 3 — Richer OpenCode Integration

Improve the reliability and power of the OpenCode integration.

- Structured JSON output via OpenCode's `format.json_schema` — replace the `PAIGE_STATUS` string protocol with a validated response object
- Context injection: include PR metadata, recent git log, branch name, etc. in the enriched prompt
- Spawn mode: Paige can start and manage its own `opencode serve` process rather than requiring one to already be running
- SSE streaming: pipe token-by-token output to the TUI job detail view via `GET /event`

---

## Milestone 4 — Notifications

Alert the user when a job requires attention without requiring the TUI to be open.

- Desktop notification (macOS `osascript`, Linux `notify-send`) when a job moves to `pending`
- Slack webhook support as an alternative notification channel
- Configurable per-job notification preferences

---

## Milestone 5 — Remote Mode (Railway)

Extend Paige so the daemon can run persistently on a server while the TUI stays local.

- The daemon grows its own HTTP API; the TUI becomes a thin client to that API
- Postgres backend (`PostgresStore` implementing the `Store` interface)
- Authentication for the HTTP API
- Railway deployment config (`railway.toml`, `Dockerfile` or `nixpacks`)
- `paige tui --server https://paige.railway.app` remote connection flag
