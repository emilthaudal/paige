# Development

## Current State

The v1 scaffold is complete and `go build ./...` passes cleanly. The following is built and functional:

- **CLI** (`cmd/paige/main.go`): Cobra commands — `serve`, `tui`, `add`, `list`
- **Domain types** (`internal/job`): `Job`, `Run`, `State` (`active`, `running`, `pending`, `completed`, `cancelled`, `paused`), `RunStatus` enums, constructors
- **Store interface + SQLite backend** (`internal/store`): Full CRUD for jobs and runs, schema migration on open
- **OpenCode HTTP client** (`internal/opencode`): Health check, create/delete session, send prompt, extract text
- **Daemon** (`internal/daemon`): gocron v2 scheduler, job execution loop, `PAIGE_STATUS` response parsing, state machine transitions, `ConfirmJob` (pending → completed), `CancelJob` (any non-terminal → cancelled)
- **TUI root model** (`internal/tui/tui.go`): Bubble Tea program wiring, view routing
- **Job list screen** (`internal/tui/joblist.go`): Async load, state icons, `r` to refresh
- **Job detail stub** (`internal/tui/jobdetail.go`): Present but non-functional (no-op Init/Update/View) — replaced in Milestone 1

---

## Milestone 0 — Fix the Scaffold ✓ Complete

All scaffold issues resolved. The tool runs end-to-end without panics or silent failures.

- Fixed `ConfirmJob` semantics: pending → completed (was incorrectly closing)
- Renamed `CloseJob` to `CancelJob`; added `StateCompleted` and `StateCancelled`, removed `StateClosed`
- Wired cancel keybind (`c`) in the TUI job list with `y/N` confirmation prompt
- Initialized `jobDetail` in root `Model` (no longer panics on navigation)
- Auto-create `~/.paige/` on startup via `os.MkdirAll` in `initServices()`
- Removed `stateLabel()` dead code and unwired `activeFilter` field
- Removed `paige close` CLI command

---

## Milestone 1 — Functional TUI ✓ Complete

A complete, navigable TUI that covers the core user workflow.

- **Job detail view**: `JobDetailModel` shows job metadata panel + full run history in a scrollable `bubbles/viewport`. Each run displays status icon, timestamps, duration, agent_done indicator, and up to 3 lines of output preview.
- **Confirm / cancel flow**: From the detail view, `enter` confirms a pending job (→ completed) and `c` cancels any non-terminal job (→ cancelled). Both show a `y/N` inline confirmation prompt before calling the daemon.
- **Navigation**: `enter` on a job list item navigates to the detail view. `esc`/`b` returns to the list. Navigation back with `refresh: true` triggers a list reload.
- **State filter tabs**: Tab bar at the top of the job list with `All / Active / Running / Pending / Completed / Cancelled / Paused`. `tab`/`shift+tab` cycles tabs and re-filters `ListJobs`.
- **Store tests**: 12 tests in `internal/store/sqlite_test.go` covering all CRUD operations.
- **Daemon tests**: 11 tests in `internal/daemon/daemon_test.go` using `mockOCClient` and `mockStore`; `OCClient` interface extracted from `opencode.Client`; `BuildPrompt` and `ParseAgentDone` exported.

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
