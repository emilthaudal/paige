# Architecture

## Overview

Paige is a local-first CLI tool that schedules AI agent tasks on a cron schedule using [OpenCode](https://opencode.ai) as its execution backend. You define a job as a prompt + repo + cron expression. Paige fires the prompt on schedule, enriches it with context, sends it to an OpenCode session, and parses the response to determine if the task is complete. When the agent believes the task is done, the job moves to a pending state and pages you for confirmation. You confirm (or dismiss) via the TUI.

---

## Core Concepts

### Job

A job is the primary entity. It has a name, a target repository, a prompt template, and a cron schedule. A job moves through a defined set of states:

```
                    ┌─────────────────────────────────────────────────────────┐
                    │                                                         │
         ┌──────────▼──────────┐    cron fires    ┌──────────────────────┐   │
         │       ACTIVE        │─────────────────►│       RUNNING        │   │
         │  scheduled, waiting │                  │  OC session in flight│   │
         └─────────────────────┘                  └──────────┬───────────┘   │
                    ▲                                         │               │
                    │  not done                               │ done          │
                    └─────────────────────────────────────────┘               │
                                                              │ done          │
                                                   ┌──────────▼───────────┐  │
                                                   │       PENDING        │  │
                                                   │  awaiting human      │  │
                                                   │  confirmation        │  │
                                                   └──────────┬───────────┘  │
                                                              │               │
                                          confirm ┌───────────┘               │
                                                  │                           │
                                       ┌──────────▼───────────┐              │
                                       │        CLOSED        │              │
                                       │  done, archived      │              │
                                       └──────────────────────┘              │
                                                                              │
         ┌─────────────────────┐                                              │
         │       PAUSED        │◄─────────────────────────────────────────────┘
         │  temporarily off    │   (manual pause from any state)
         └─────────────────────┘
```

### Run

A run is a single execution of a job. Every time the cron fires and a job executes, a run record is created. It tracks the OpenCode session ID, start/end times, the full output, and whether the agent reported the task as done. A job has many runs.

### OpenCode Session

Each run creates a fresh OpenCode session via the OpenCode HTTP API, sends an enriched prompt, waits for the response, then deletes the session. The prompt includes the repository context and a suffix that instructs the agent to report a `PAIGE_STATUS` marker. Future versions will use OpenCode's structured JSON output instead.

---

## System Diagram

```
┌────────────────────────────────────────────────────────────────┐
│  paige CLI (cobra)                                             │
│                                                                │
│  paige tui      ──► starts daemon in goroutine + opens TUI    │
│  paige serve    ──► starts daemon only (headless)             │
│  paige add      ──► creates a job in the store                │
│  paige list     ──► reads jobs from the store                 │
│  paige close    ──► closes a job via the daemon               │
└───────────┬──────────────────────────────┬─────────────────────┘
            │                              │
┌───────────▼──────────┐       ┌───────────▼──────────────────────┐
│  Daemon              │       │  TUI (Bubble Tea)                │
│                      │       │                                  │
│  - gocron v2         │       │  - Job list (all states)         │
│  - job state machine │       │  - Job detail + run history      │
│  - executeJob loop   │       │  - Confirm / close pending jobs  │
│  - singleton mode    │       │  - State filter tabs             │
│    (no overlap)      │       │  - Interactive add wizard        │
└───────────┬──────────┘       └──────────────────────────────────┘
            │                              │
            │         ┌────────────────────┘
            │         │
┌───────────▼──────────▼───────┐
│  Store (interface)           │
│                              │
│  SQLiteStore (local, v1)     │
│  PostgresStore (Railway, v2) │
└───────────┬──────────────────┘
            │
┌───────────▼──────────────────┐
│  SQLite DB (~/.paige/paige.db)│
│                              │
│  jobs   runs                 │
└──────────────────────────────┘

            │ (separate process)
┌───────────▼──────────────────┐
│  OpenCode Client             │
│                              │
│  POST /session               │
│  POST /session/:id/message   │
│  DELETE /session/:id         │
│  GET /global/health          │
└───────────┬──────────────────┘
            │
┌───────────▼──────────────────┐
│  OpenCode server             │
│  (opencode serve)            │
│                              │
│  Manages LLM sessions,       │
│  tools, file access          │
└──────────────────────────────┘
```

---

## Package Map

### `internal/job`

The domain package. Contains no I/O, no dependencies outside stdlib. Defines the core types and constructors.

| Type / Function | Purpose |
|---|---|
| `Job` | Primary entity: name, repo, prompt, schedule, state |
| `Run` | Single execution record for a job |
| `State` | Enum: `active`, `running`, `pending`, `closed`, `paused` |
| `RunStatus` | Enum: `running`, `done`, `failed` |
| `NewJob(...)` | Constructor — sets UUID, defaults to `StateActive` |
| `NewRun(...)` | Constructor — sets UUID, defaults to `RunStatusRunning` |

### `internal/store`

Persistence layer. The `Store` interface is the only boundary the rest of the app talks to. The SQLite implementation is the current backend.

| Type / Function | Purpose |
|---|---|
| `Store` | Interface: CRUD for jobs and runs, `Close()` |
| `JobFilter` | Controls which states are returned by `ListJobs` |
| `SQLiteStore` | SQLite implementation via `modernc.org/sqlite` (pure Go, no CGO) |
| `NewSQLiteStore(path)` | Opens/creates DB, runs schema migration |

### `internal/opencode`

HTTP client for the OpenCode server API. Stateless — no caching, no session reuse.

| Type / Function | Purpose |
|---|---|
| `Client` | HTTP client with base URL and timeout |
| `NewClient(opts...)` | Constructor with functional options |
| `Health(ctx)` | Checks if the OpenCode server is reachable |
| `CreateSession(ctx, title)` | Creates a new OpenCode session |
| `SendPrompt(ctx, sessionID, prompt)` | Sends a prompt, blocks until response |
| `DeleteSession(ctx, sessionID)` | Cleans up a session after use |
| `ExtractText(mr)` | Concatenates all text parts from a response |

### `internal/daemon`

The scheduler and execution engine. Owns the gocron instance and job lifecycle.

| Type / Function | Purpose |
|---|---|
| `Daemon` | Holds the scheduler, store ref, OC client, and live job map |
| `New(store, oc)` | Constructor |
| `Start(ctx)` | Loads active jobs, starts scheduler, blocks until ctx cancelled |
| `RegisterJob(ctx, job)` | Persists job and adds it to the live scheduler |
| `ConfirmJob(ctx, id)` | Transitions a pending job to closed |
| `CloseJob(ctx, id)` | Closes a job and removes it from the scheduler |
| `executeJob(jobID)` | Core execution: create OC session → send prompt → parse status → update state |

### `internal/tui`

The terminal UI. Built with Bubble Tea. Views are separate models composed by the root `Model`.

| Type / Function | Purpose |
|---|---|
| `Model` | Root model: owns current view, delegates to child models |
| `Run(daemon, store)` | Entry point — starts the Bubble Tea program with alt-screen |
| `JobListModel` | Job list screen with async load, state icons, `r` to refresh |
| `JobDetailModel` | Job detail + run history (stub) |

### `cmd/paige`

Entry point. Wires together cobra commands with the internal packages via `initServices()`.

---

## Data Model

### `jobs` table

```sql
CREATE TABLE jobs (
    id          TEXT PRIMARY KEY,       -- UUID
    name        TEXT NOT NULL,
    repo        TEXT NOT NULL,          -- e.g. "github.com/org/repo"
    prompt      TEXT NOT NULL,          -- user-defined prompt template
    schedule    TEXT NOT NULL,          -- cron expression
    state       TEXT NOT NULL DEFAULT 'active',
    created_at  DATETIME NOT NULL,
    updated_at  DATETIME NOT NULL
);
```

### `runs` table

```sql
CREATE TABLE runs (
    id             TEXT PRIMARY KEY,    -- UUID
    job_id         TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    started_at     DATETIME NOT NULL,
    ended_at       DATETIME,            -- null while running
    oc_session_id  TEXT NOT NULL DEFAULT '',
    output         TEXT NOT NULL DEFAULT '',
    status         TEXT NOT NULL DEFAULT 'running',
    agent_done     INTEGER NOT NULL DEFAULT 0  -- bool: did OC report PAIGE_STATUS: done
);
```

---

## OpenCode Integration

### Connection model

Paige connects to an **already-running** OpenCode server. The user is expected to have `opencode serve` running (or a full `opencode` TUI session, which also starts a server). The server URL defaults to `http://localhost:4096` and is configurable via `--opencode-url`.

A future mode will allow Paige to **spawn and manage** an `opencode serve` process itself, tracking its PID and port.

### Prompt enrichment

Before sending a prompt to OpenCode, the daemon builds an enriched version:

```
Repository: <repo>

<user prompt>

---
At the end of your response, include a line in exactly this format:
PAIGE_STATUS: done
or
PAIGE_STATUS: not_done
```

The `PAIGE_STATUS` line is how Paige determines whether to move a job to `pending` (awaiting confirmation) or leave it `active` for the next tick.

### Future: structured output

The OpenCode API supports a `format.json_schema` field on prompt requests that returns a validated JSON object as `structured_output` on the message. Paige will migrate to this for more reliable status reporting, replacing the `PAIGE_STATUS` string protocol.

---

## Local vs. Remote Architecture

The current design is **local-first**: the daemon, store, and TUI all run in the same process on the same machine. The `Store` interface is the seam that makes remote mode possible without rewriting the core logic.

### V1 — Local (current)

```
paige tui
  └── daemon (goroutine)
  └── SQLiteStore (~/.paige/paige.db)
  └── TUI (same process)
```

### V2 — Remote (Railway)

```
[Railway]                          [Local machine]
paige-server                       paige tui --server https://paige.railway.app
  └── daemon                         └── HTTP client → paige-server API
  └── PostgresStore                  └── TUI renders remote state
  └── HTTP API (own REST API)
```

The key change: the daemon grows its own HTTP API. The TUI becomes a client to that API rather than calling `store` and `daemon` directly. Locally, the API runs in-process (no network hop). Remotely, it's a separate service.

---

## Key Design Decisions

| Decision | Rationale |
|---|---|
| `modernc.org/sqlite` (pure Go) | No CGO required, so `go build` works without a C toolchain. Easy to swap to Postgres later by implementing `Store` for a different driver. |
| `gocron/v2` with `LimitModeReschedule` | Prevents overlapping executions of the same job. If a tick fires while the previous run is still in flight, the new tick is dropped rather than queued, avoiding runaway parallelism. |
| `PAIGE_STATUS` string protocol | Simple to implement and debug. Relies on the LLM following instructions, which is imperfect. Structured JSON output (via OpenCode's `format.json_schema`) is the planned replacement. |
| `Store` interface at the package boundary | Decouples all business logic from the storage backend. The daemon, TUI, and CLI never import a concrete store — they only depend on the interface. |
| Daemon + TUI in same process (v1) | Simplest path to a working product. No IPC, no sockets. The split into a separate HTTP API is a deliberate future step, not a current constraint. |
| Cobra for CLI | Subcommand structure (`serve`, `tui`, `add`, `list`, `close`) maps cleanly to `cobra.Command`. Well-documented, widely used in Go tooling. |
