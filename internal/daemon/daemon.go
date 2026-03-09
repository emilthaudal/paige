// Package daemon manages the Paige scheduler: loading jobs from the store,
// registering them with gocron, executing them against OpenCode, and
// advancing job state.
package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"

	"github.com/emtb/paige/internal/job"
	"github.com/emtb/paige/internal/opencode"
	"github.com/emtb/paige/internal/store"
)

// systemPromptSuffix is appended to every job prompt so OpenCode knows to
// report back a structured completion status.
const systemPromptSuffix = `

---
At the end of your response, include a line in exactly this format:
PAIGE_STATUS: done
or
PAIGE_STATUS: not_done

Use "done" only if you believe the task described above is fully complete.
Use "not_done" if the task is ongoing or requires further work.
`

// Daemon is the background scheduler service.
type Daemon struct {
	store     store.Store
	oc        opencode.OCClient
	scheduler gocron.Scheduler
	mu        sync.Mutex
	jobs      map[uuid.UUID]gocron.Job // gocron job handles by Paige job ID
}

// New creates a new Daemon. Call Start to begin scheduling.
func New(st store.Store, oc opencode.OCClient) (*Daemon, error) {
	s, err := gocron.NewScheduler()
	if err != nil {
		return nil, fmt.Errorf("create scheduler: %w", err)
	}
	return &Daemon{
		store:     st,
		oc:        oc,
		scheduler: s,
		jobs:      make(map[uuid.UUID]gocron.Job),
	}, nil
}

// Start loads all active jobs from the store and begins the scheduler loop.
func (d *Daemon) Start(ctx context.Context) error {
	active, err := d.store.ListJobs(ctx, store.JobFilter{
		States: []job.State{job.StateActive},
	})
	if err != nil {
		return fmt.Errorf("load jobs: %w", err)
	}

	for _, j := range active {
		if err := d.scheduleJob(j); err != nil {
			slog.Error("failed to schedule job", "job", j.Name, "err", err)
		}
	}

	d.scheduler.Start()
	slog.Info("daemon started", "active_jobs", len(active))

	<-ctx.Done()
	return d.scheduler.Shutdown()
}

// RegisterJob adds a job to both the store and the live scheduler.
func (d *Daemon) RegisterJob(ctx context.Context, j job.Job) error {
	if err := d.store.CreateJob(ctx, j); err != nil {
		return fmt.Errorf("store job: %w", err)
	}
	return d.scheduleJob(j)
}

// ConfirmJob transitions a pending job to completed (human confirmed agent is done).
// The job is unscheduled — completed is a terminal state.
func (d *Daemon) ConfirmJob(ctx context.Context, id uuid.UUID) error {
	j, err := d.store.GetJob(ctx, id)
	if err != nil {
		return err
	}
	if j.State != job.StatePending {
		return fmt.Errorf("job %s is not pending (state: %s)", id, j.State)
	}
	j.State = job.StateCompleted
	if err := d.store.UpdateJob(ctx, j); err != nil {
		return err
	}
	return d.unscheduleJob(id)
}

// CancelJob moves a job to the cancelled state and removes it from the scheduler.
// It may be called from any non-terminal state (active, running, pending, paused).
func (d *Daemon) CancelJob(ctx context.Context, id uuid.UUID) error {
	j, err := d.store.GetJob(ctx, id)
	if err != nil {
		return err
	}
	if j.State == job.StateCompleted || j.State == job.StateCancelled {
		return fmt.Errorf("job %s is already terminal (state: %s)", id, j.State)
	}
	j.State = job.StateCancelled
	if err := d.store.UpdateJob(ctx, j); err != nil {
		return err
	}
	return d.unscheduleJob(id)
}

// TriggerJob immediately executes a job, bypassing its cron schedule.
// This is primarily useful for testing.
func (d *Daemon) TriggerJob(id uuid.UUID) {
	d.executeJob(id)
}
func (d *Daemon) scheduleJob(j job.Job) error {
	gj, err := d.scheduler.NewJob(
		gocron.CronJob(j.Schedule, false),
		gocron.NewTask(d.executeJob, j.ID),
		gocron.WithName(j.ID.String()),
		gocron.WithSingletonMode(gocron.LimitModeReschedule),
	)
	if err != nil {
		return fmt.Errorf("register cron job %s: %w", j.Name, err)
	}

	d.mu.Lock()
	d.jobs[j.ID] = gj
	d.mu.Unlock()

	slog.Info("job scheduled", "name", j.Name, "schedule", j.Schedule)
	return nil
}

// unscheduleJob removes a job from the live scheduler.
func (d *Daemon) unscheduleJob(id uuid.UUID) error {
	d.mu.Lock()
	gj, ok := d.jobs[id]
	d.mu.Unlock()
	if !ok {
		return nil
	}
	if err := d.scheduler.RemoveJob(gj.ID()); err != nil {
		return err
	}
	d.mu.Lock()
	delete(d.jobs, id)
	d.mu.Unlock()
	return nil
}

// executeJob is called by gocron on each tick. It creates an OpenCode session,
// sends the enriched prompt, and advances job state based on the response.
func (d *Daemon) executeJob(jobID uuid.UUID) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	j, err := d.store.GetJob(ctx, jobID)
	if err != nil {
		slog.Error("execute: load job", "id", jobID, "err", err)
		return
	}

	// Don't run if already running or pending confirmation.
	if j.State == job.StateRunning || j.State == job.StatePending {
		slog.Info("execute: skipping, job not in runnable state", "name", j.Name, "state", j.State)
		return
	}

	slog.Info("execute: starting", "name", j.Name)

	// Transition to running.
	j.State = job.StateRunning
	if err := d.store.UpdateJob(ctx, j); err != nil {
		slog.Error("execute: update to running", "err", err)
		return
	}

	// Create an OpenCode session.
	session, err := d.oc.CreateSession(ctx, fmt.Sprintf("paige: %s", j.Name))
	if err != nil {
		slog.Error("execute: create OC session", "err", err)
		d.failJob(ctx, j)
		return
	}

	// Record the run.
	r := job.NewRun(j.ID, session.ID)
	if err := d.store.CreateRun(ctx, r); err != nil {
		slog.Error("execute: create run", "err", err)
	}

	// Build the enriched prompt.
	prompt := BuildPrompt(j)

	// Send to OpenCode.
	resp, err := d.oc.SendPrompt(ctx, session.ID, prompt)
	if err != nil {
		slog.Error("execute: send prompt", "err", err)
		d.failRun(ctx, r, "")
		d.failJob(ctx, j)
		return
	}

	output := opencode.ExtractText(resp)
	agentDone := ParseAgentDone(output)

	// Finalize the run.
	now := time.Now().UTC()
	r.EndedAt = &now
	r.Output = output
	r.AgentDone = agentDone
	r.Status = job.RunStatusDone
	if err := d.store.UpdateRun(ctx, r); err != nil {
		slog.Error("execute: update run", "err", err)
	}

	// Advance job state.
	if agentDone {
		j.State = job.StatePending // awaiting human confirmation
		slog.Info("execute: agent reports done, job now pending", "name", j.Name)
	} else {
		j.State = job.StateActive // back to active, will run again on schedule
		slog.Info("execute: agent reports not done, job remains active", "name", j.Name)
	}
	if err := d.store.UpdateJob(ctx, j); err != nil {
		slog.Error("execute: finalize job state", "err", err)
	}

	// Clean up the OC session.
	_ = d.oc.DeleteSession(context.Background(), session.ID)
}

// BuildPrompt constructs the full prompt to send to OpenCode.
func BuildPrompt(j job.Job) string {
	return fmt.Sprintf("Repository: %s\n\n%s%s", j.Repo, j.Prompt, systemPromptSuffix)
}

// ParseAgentDone looks for the PAIGE_STATUS marker in the output.
func ParseAgentDone(output string) bool {
	return strings.Contains(output, "PAIGE_STATUS: done")
}

func (d *Daemon) failJob(ctx context.Context, j job.Job) {
	j.State = job.StateActive
	if err := d.store.UpdateJob(ctx, j); err != nil {
		slog.Error("fail: reset job to active", "err", err)
	}
}

func (d *Daemon) failRun(ctx context.Context, r job.Run, output string) {
	now := time.Now().UTC()
	r.EndedAt = &now
	r.Status = job.RunStatusFailed
	r.Output = output
	if err := d.store.UpdateRun(ctx, r); err != nil {
		slog.Error("fail: update run", "err", err)
	}
}
