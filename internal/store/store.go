// Package store defines the persistence interface for Paige.
// The interface is designed to be backend-agnostic: SQLite locally,
// Postgres on Railway in the future.
package store

import (
	"context"

	"github.com/google/uuid"

	"github.com/emtb/paige/internal/job"
)

// Store is the unified persistence interface for jobs and runs.
type Store interface {
	// Job operations
	CreateJob(ctx context.Context, j job.Job) error
	GetJob(ctx context.Context, id uuid.UUID) (job.Job, error)
	ListJobs(ctx context.Context, filter JobFilter) ([]job.Job, error)
	UpdateJob(ctx context.Context, j job.Job) error
	DeleteJob(ctx context.Context, id uuid.UUID) error

	// Run operations
	CreateRun(ctx context.Context, r job.Run) error
	GetRun(ctx context.Context, id uuid.UUID) (job.Run, error)
	ListRuns(ctx context.Context, jobID uuid.UUID) ([]job.Run, error)
	UpdateRun(ctx context.Context, r job.Run) error
	LatestRun(ctx context.Context, jobID uuid.UUID) (job.Run, error)

	// Lifecycle
	Close() error
}

// JobFilter controls which jobs are returned by ListJobs.
type JobFilter struct {
	// States filters to only jobs in the given states.
	// If empty, all states are returned.
	States []job.State
}
