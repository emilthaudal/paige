// Package job defines the core domain types for Paige: jobs, runs, and their
// state machines.
package job

import (
	"time"

	"github.com/google/uuid"
)

// State represents the lifecycle state of a Job.
type State string

const (
	// StateActive means the job is scheduled and will fire on its cron schedule.
	StateActive State = "active"
	// StateRunning means an OpenCode session is currently executing for this job.
	StateRunning State = "running"
	// StatePending means OpenCode reported the task is done; awaiting human confirmation.
	StatePending State = "pending"
	// StateCompleted means the human confirmed the agent's "done" signal. Terminal.
	StateCompleted State = "completed"
	// StateCancelled means the user explicitly stopped the job. Terminal.
	StateCancelled State = "cancelled"
	// StatePaused means the job is temporarily disabled.
	StatePaused State = "paused"
)

// RunStatus represents the outcome of a single job execution.
type RunStatus string

const (
	RunStatusRunning RunStatus = "running"
	RunStatusDone    RunStatus = "done"
	RunStatusFailed  RunStatus = "failed"
)

// Job is the core entity: a prompt on a schedule tied to a repo.
type Job struct {
	ID        uuid.UUID `db:"id"`
	Name      string    `db:"name"`
	Repo      string    `db:"repo"`     // e.g. "github.com/user/repo" or local path
	Prompt    string    `db:"prompt"`   // user-defined prompt template
	Schedule  string    `db:"schedule"` // cron expression, e.g. "*/5 * * * *"
	State     State     `db:"state"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

// Run represents a single execution of a Job.
type Run struct {
	ID          uuid.UUID  `db:"id"`
	JobID       uuid.UUID  `db:"job_id"`
	StartedAt   time.Time  `db:"started_at"`
	EndedAt     *time.Time `db:"ended_at"`
	OCSessionID string     `db:"oc_session_id"` // OpenCode session ID
	Output      string     `db:"output"`        // full OpenCode response text
	Status      RunStatus  `db:"status"`
	AgentDone   bool       `db:"agent_done"` // true if OC reported task complete
}

// NewJob creates a Job with a new UUID and defaults.
func NewJob(name, repo, prompt, schedule string) Job {
	now := time.Now().UTC()
	return Job{
		ID:        uuid.New(),
		Name:      name,
		Repo:      repo,
		Prompt:    prompt,
		Schedule:  schedule,
		State:     StateActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// NewRun creates a Run for the given job with a new UUID.
func NewRun(jobID uuid.UUID, ocSessionID string) Run {
	return Run{
		ID:          uuid.New(),
		JobID:       jobID,
		StartedAt:   time.Now().UTC(),
		OCSessionID: ocSessionID,
		Status:      RunStatusRunning,
	}
}
