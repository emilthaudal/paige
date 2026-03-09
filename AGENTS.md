# Agent Instructions

Paige is a **Go CLI/TUI application** — a cron-based AI job orchestrator that runs OpenCode sessions on a schedule. It uses Cobra for CLI, Bubble Tea for the TUI, gocron for scheduling, and SQLite for persistence.

This project uses **bd** (beads) for issue tracking. Run `bd onboard` to get started.

## Build & Test Commands

```bash
make build      # Compile binary to ./paige
make test       # Run all tests: go test ./...
make lint       # Run golangci-lint (must be installed)
make tidy       # go mod tidy
make clean      # Remove build artifacts
make install    # Install to $GOPATH/bin
```

**Run a single test by name:**
```bash
go test ./internal/job/... -run TestFunctionName
go test -v ./internal/store/... -run TestSQLiteStore
```

**Run tests for a specific package:**
```bash
go test ./internal/job/...
go test ./internal/store/...
go test ./internal/daemon/...
```

**Build with version info:**
```bash
go build -ldflags "-X main.version=$(git describe --tags --always)" -o paige ./cmd/paige
```

## Code Style Guidelines

### Language & Tooling
- **Go 1.26.1** — follow idiomatic Go conventions
- Linter: `golangci-lint` (no config file; uses defaults)
- Formatter: `gofmt` / `goimports`

### Import Grouping
Always use three groups separated by blank lines:
```go
import (
    // 1. stdlib
    "context"
    "fmt"
    "time"

    // 2. third-party
    "github.com/charmbracelet/bubbletea"
    "github.com/google/uuid"

    // 3. internal
    "github.com/emtb/paige/internal/job"
    "github.com/emtb/paige/internal/store"
)
```

### Naming Conventions
- **Packages**: short, lowercase, single word — `job`, `store`, `daemon`, `tui`, `opencode`
- **Types**: `PascalCase` — `Job`, `Run`, `State`, `SQLiteStore`, `JobListModel`
- **Unexported helpers**: `camelCase` — `scanJob`, `scanRun`, `stateIcon`, `buildPrompt`
- **Enum constants**: `TypeValue` pattern — `StateActive`, `StateRunning`, `RunStatusDone`
- **Constructors**: `New<Type>(...)` — `NewJob(...)`, `NewRun(...)`, `NewSQLiteStore(...)`
- **Functional option functions**: `With<Thing>(...)` — `WithBaseURL(...)`, `WithTimeout(...)`

### Error Handling
- Always wrap errors with context: `fmt.Errorf("create job: %w", err)`
- Use `RunE` (not `Run`) in Cobra commands so errors propagate correctly
- Log with structured `log/slog`: `slog.Error("msg", "key", val, "err", err)`
- Functions that can fail return `(value, error)` — never panic on recoverable errors
- Do not discard errors; always check the returned `error`

### Package & File Conventions
- Every file starts with a package doc comment:
  ```go
  // Package job defines the core domain types for Paige: jobs, runs, and
  // their state machines.
  package job
  ```
- Every exported type, function, and constant has a doc comment
- All times stored/returned as UTC: `time.Now().UTC()`
- Struct fields use `db:"column_name"` tags for SQLite scanning

### Architecture Patterns
- **Interface-driven**: depend on `store.Store` (the interface), not `*SQLiteStore` (the impl)
- **Functional options** for client configuration — `opencode.WithBaseURL(url)`
- **Bubble Tea MVU**: each TUI view is a model with `Init() / Update() / View()` methods
- **Context everywhere**: all store and HTTP operations accept `context.Context` as first param
- `cmd/paige` wires services together via `initServices()` — no business logic in `cmd/`
- `internal/job` is a **pure domain package**: no I/O, no external dependencies beyond `uuid`
- Third-party imports (e.g., `modernc.org/sqlite`) stay inside their owning package — nothing outside `store/` imports the SQLite driver directly

### Common Pitfalls to Avoid
- Do not initialize `~/.paige/` manually — the app creates it on first run
- The `jobDetail` model is scaffolded but not functional; navigating to it will panic
- `ConfirmJob` in the TUI currently closes jobs instead of confirming them (known bug)

## Non-Interactive Shell Commands

**ALWAYS use non-interactive flags** with file operations to avoid hanging on confirmation prompts.

Shell commands like `cp`, `mv`, and `rm` may be aliased to include `-i` (interactive) mode on some systems, causing the agent to hang indefinitely waiting for y/n input.

**Use these forms instead:**
```bash
cp -f source dest           # NOT: cp source dest
mv -f source dest           # NOT: mv source dest
rm -f file                  # NOT: rm file
rm -rf directory            # NOT: rm -r directory
cp -rf source dest          # NOT: cp -r source dest
```

**Other commands that may prompt:**
- `scp` — use `-o BatchMode=yes`
- `ssh` — use `-o BatchMode=yes`
- `apt-get` — use `-y` flag
- `brew` — use `HOMEBREW_NO_AUTO_UPDATE=1`

<!-- BEGIN BEADS INTEGRATION -->
## Issue Tracking with bd (beads)

**IMPORTANT**: This project uses **bd (beads)** for ALL issue tracking. Do NOT use markdown TODOs, task lists, or other tracking methods.

**Check for ready work:**
```bash
bd ready --json
```

**Create new issues:**
```bash
bd create "Issue title" --description="Detailed context" -t bug|feature|task -p 0-4 --json
bd create "Issue title" --description="Details" -p 1 --deps discovered-from:bd-123 --json
```

**Claim and update:**
```bash
bd update <id> --claim --json
bd update bd-42 --priority 1 --json
```

**Complete work:**
```bash
bd close bd-42 --reason "Completed" --json
```

### Issue Types & Priorities

Types: `bug` · `feature` · `task` · `epic` · `chore`

Priorities: `0` critical · `1` high · `2` medium (default) · `3` low · `4` backlog

### Workflow for AI Agents

1. **Check ready work**: `bd ready` shows unblocked issues
2. **Claim your task atomically**: `bd update <id> --claim`
3. **Work on it**: implement, test, document
4. **Discover new work?** `bd create "Found bug" -p 1 --deps discovered-from:<parent-id>`
5. **Complete**: `bd close <id> --reason "Done"`

### Important Rules

- Use bd for ALL task tracking — never markdown TODOs
- Always use `--json` flag for programmatic use
- Link discovered work with `discovered-from` dependencies
- Check `bd ready` before asking "what should I work on?"

<!-- END BEADS INTEGRATION -->

## Landing the Plane (Session Completion)

**When ending a work session**, complete ALL steps below. Work is NOT complete until `git push` succeeds.

1. **File issues for remaining work** — create bd issues for any follow-up
2. **Run quality gates** (if code changed): `make test && make lint && make build`
3. **Update issue status** — close finished work, update in-progress items
4. **Push to remote** — MANDATORY:
   ```bash
   git pull --rebase
   bd sync
   git push
   git status  # MUST show "up to date with origin"
   ```
5. **Verify** — all changes committed AND pushed
6. **Hand off** — provide context for the next session

**Critical rules:**
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing — that leaves work stranded locally
- NEVER say "ready to push when you are" — YOU must push
- If push fails, resolve and retry until it succeeds
