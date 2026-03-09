package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite" // register the sqlite driver

	"github.com/emtb/paige/internal/job"
)

const schema = `
CREATE TABLE IF NOT EXISTS jobs (
	id          TEXT PRIMARY KEY,
	name        TEXT NOT NULL,
	repo        TEXT NOT NULL,
	prompt      TEXT NOT NULL,
	schedule    TEXT NOT NULL,
	state       TEXT NOT NULL DEFAULT 'active',
	created_at  DATETIME NOT NULL,
	updated_at  DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS runs (
	id             TEXT PRIMARY KEY,
	job_id         TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
	started_at     DATETIME NOT NULL,
	ended_at       DATETIME,
	oc_session_id  TEXT NOT NULL DEFAULT '',
	output         TEXT NOT NULL DEFAULT '',
	status         TEXT NOT NULL DEFAULT 'running',
	agent_done     INTEGER NOT NULL DEFAULT 0,
	FOREIGN KEY (job_id) REFERENCES jobs(id)
);

CREATE INDEX IF NOT EXISTS idx_runs_job_id ON runs(job_id);
CREATE INDEX IF NOT EXISTS idx_jobs_state  ON jobs(state);
`

// SQLiteStore is a Store backed by a local SQLite database.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore opens (or creates) the SQLite database at the given path
// and runs the schema migration.
func NewSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite is single-writer

	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("migrate schema: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

// Close closes the underlying database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// --- Job operations ---

func (s *SQLiteStore) CreateJob(ctx context.Context, j job.Job) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO jobs (id, name, repo, prompt, schedule, state, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		j.ID.String(), j.Name, j.Repo, j.Prompt, j.Schedule,
		string(j.State), j.CreatedAt.UTC(), j.UpdatedAt.UTC(),
	)
	return err
}

func (s *SQLiteStore) GetJob(ctx context.Context, id uuid.UUID) (job.Job, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, repo, prompt, schedule, state, created_at, updated_at
		 FROM jobs WHERE id = ?`, id.String())
	return scanJob(row)
}

func (s *SQLiteStore) ListJobs(ctx context.Context, filter JobFilter) ([]job.Job, error) {
	query := `SELECT id, name, repo, prompt, schedule, state, created_at, updated_at FROM jobs`
	var args []any

	if len(filter.States) > 0 {
		query += ` WHERE state IN (`
		for i, st := range filter.States {
			if i > 0 {
				query += ","
			}
			query += "?"
			args = append(args, string(st))
		}
		query += ")"
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []job.Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

func (s *SQLiteStore) UpdateJob(ctx context.Context, j job.Job) error {
	j.UpdatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`UPDATE jobs SET name=?, repo=?, prompt=?, schedule=?, state=?, updated_at=?
		 WHERE id=?`,
		j.Name, j.Repo, j.Prompt, j.Schedule, string(j.State), j.UpdatedAt, j.ID.String(),
	)
	return err
}

func (s *SQLiteStore) DeleteJob(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM jobs WHERE id = ?`, id.String())
	return err
}

// --- Run operations ---

func (s *SQLiteStore) CreateRun(ctx context.Context, r job.Run) error {
	agentDone := 0
	if r.AgentDone {
		agentDone = 1
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO runs (id, job_id, started_at, ended_at, oc_session_id, output, status, agent_done)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID.String(), r.JobID.String(), r.StartedAt.UTC(), r.EndedAt,
		r.OCSessionID, r.Output, string(r.Status), agentDone,
	)
	return err
}

func (s *SQLiteStore) GetRun(ctx context.Context, id uuid.UUID) (job.Run, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, job_id, started_at, ended_at, oc_session_id, output, status, agent_done
		 FROM runs WHERE id = ?`, id.String())
	return scanRun(row)
}

func (s *SQLiteStore) ListRuns(ctx context.Context, jobID uuid.UUID) ([]job.Run, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, job_id, started_at, ended_at, oc_session_id, output, status, agent_done
		 FROM runs WHERE job_id = ? ORDER BY started_at DESC`, jobID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []job.Run
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

func (s *SQLiteStore) UpdateRun(ctx context.Context, r job.Run) error {
	agentDone := 0
	if r.AgentDone {
		agentDone = 1
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE runs SET ended_at=?, oc_session_id=?, output=?, status=?, agent_done=?
		 WHERE id=?`,
		r.EndedAt, r.OCSessionID, r.Output, string(r.Status), agentDone, r.ID.String(),
	)
	return err
}

func (s *SQLiteStore) LatestRun(ctx context.Context, jobID uuid.UUID) (job.Run, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, job_id, started_at, ended_at, oc_session_id, output, status, agent_done
		 FROM runs WHERE job_id = ? ORDER BY started_at DESC LIMIT 1`, jobID.String())
	return scanRun(row)
}

// --- scan helpers ---

type scanner interface {
	Scan(dest ...any) error
}

func scanJob(s scanner) (job.Job, error) {
	var j job.Job
	var id, state string
	var createdAt, updatedAt time.Time

	err := s.Scan(&id, &j.Name, &j.Repo, &j.Prompt, &j.Schedule,
		&state, &createdAt, &updatedAt)
	if err != nil {
		return job.Job{}, err
	}
	j.ID, _ = uuid.Parse(id)
	j.State = job.State(state)
	j.CreatedAt = createdAt
	j.UpdatedAt = updatedAt
	return j, nil
}

func scanRun(s scanner) (job.Run, error) {
	var r job.Run
	var id, jobID, status string
	var agentDone int
	var endedAt sql.NullTime

	err := s.Scan(&id, &jobID, &r.StartedAt, &endedAt,
		&r.OCSessionID, &r.Output, &status, &agentDone)
	if err != nil {
		return job.Run{}, err
	}
	r.ID, _ = uuid.Parse(id)
	r.JobID, _ = uuid.Parse(jobID)
	r.Status = job.RunStatus(status)
	r.AgentDone = agentDone == 1
	if endedAt.Valid {
		r.EndedAt = &endedAt.Time
	}
	return r, nil
}
