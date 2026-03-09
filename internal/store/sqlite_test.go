package store_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/emtb/paige/internal/job"
	"github.com/emtb/paige/internal/store"
)

func newTestStore(t *testing.T) *store.SQLiteStore {
	t.Helper()
	s, err := store.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func newJob(name string) job.Job {
	return job.NewJob(name, "github.com/test/repo", "do the thing", "*/5 * * * *")
}

// --- Job tests ---

func TestCreateAndGetJob(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	j := newJob("my-job")
	if err := s.CreateJob(ctx, j); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	got, err := s.GetJob(ctx, j.ID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}

	if got.ID != j.ID {
		t.Errorf("ID: got %v, want %v", got.ID, j.ID)
	}
	if got.Name != j.Name {
		t.Errorf("Name: got %q, want %q", got.Name, j.Name)
	}
	if got.Repo != j.Repo {
		t.Errorf("Repo: got %q, want %q", got.Repo, j.Repo)
	}
	if got.Prompt != j.Prompt {
		t.Errorf("Prompt: got %q, want %q", got.Prompt, j.Prompt)
	}
	if got.Schedule != j.Schedule {
		t.Errorf("Schedule: got %q, want %q", got.Schedule, j.Schedule)
	}
	if got.State != job.StateActive {
		t.Errorf("State: got %v, want %v", got.State, job.StateActive)
	}
}

func TestGetJobNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	_, err := s.GetJob(ctx, uuid.New())
	if err == nil {
		t.Fatal("expected error for missing job, got nil")
	}
}

func TestListJobsNoFilter(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	for _, name := range []string{"job-a", "job-b", "job-c"} {
		if err := s.CreateJob(ctx, newJob(name)); err != nil {
			t.Fatalf("CreateJob %s: %v", name, err)
		}
	}

	jobs, err := s.ListJobs(ctx, store.JobFilter{})
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if len(jobs) != 3 {
		t.Errorf("len: got %d, want 3", len(jobs))
	}
}

func TestListJobsWithStateFilter(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	active := newJob("active-job")
	if err := s.CreateJob(ctx, active); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	pending := newJob("pending-job")
	pending.State = job.StatePending
	// Override state before creation is not possible via NewJob; create then update.
	if err := s.CreateJob(ctx, pending); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	pending.State = job.StatePending
	if err := s.UpdateJob(ctx, pending); err != nil {
		t.Fatalf("UpdateJob to pending: %v", err)
	}

	// Filter by active only.
	activeJobs, err := s.ListJobs(ctx, store.JobFilter{States: []job.State{job.StateActive}})
	if err != nil {
		t.Fatalf("ListJobs(active): %v", err)
	}
	if len(activeJobs) != 1 {
		t.Errorf("active filter: got %d jobs, want 1", len(activeJobs))
	}
	if activeJobs[0].Name != "active-job" {
		t.Errorf("active filter: got name %q, want %q", activeJobs[0].Name, "active-job")
	}

	// Filter by pending only.
	pendingJobs, err := s.ListJobs(ctx, store.JobFilter{States: []job.State{job.StatePending}})
	if err != nil {
		t.Fatalf("ListJobs(pending): %v", err)
	}
	if len(pendingJobs) != 1 {
		t.Errorf("pending filter: got %d jobs, want 1", len(pendingJobs))
	}

	// Filter by both states.
	both, err := s.ListJobs(ctx, store.JobFilter{States: []job.State{job.StateActive, job.StatePending}})
	if err != nil {
		t.Fatalf("ListJobs(active+pending): %v", err)
	}
	if len(both) != 2 {
		t.Errorf("multi-state filter: got %d jobs, want 2", len(both))
	}
}

func TestUpdateJobStatePersisted(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	j := newJob("state-test")
	if err := s.CreateJob(ctx, j); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	j.State = job.StatePending
	if err := s.UpdateJob(ctx, j); err != nil {
		t.Fatalf("UpdateJob: %v", err)
	}

	got, err := s.GetJob(ctx, j.ID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if got.State != job.StatePending {
		t.Errorf("State after update: got %v, want %v", got.State, job.StatePending)
	}
}

func TestUpdateJobAdvancesUpdatedAt(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	j := newJob("time-test")
	if err := s.CreateJob(ctx, j); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	time.Sleep(2 * time.Millisecond) // ensure time advances

	j.State = job.StateRunning
	if err := s.UpdateJob(ctx, j); err != nil {
		t.Fatalf("UpdateJob: %v", err)
	}

	got, err := s.GetJob(ctx, j.ID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if !got.UpdatedAt.After(j.CreatedAt) {
		t.Errorf("UpdatedAt %v should be after CreatedAt %v", got.UpdatedAt, j.CreatedAt)
	}
}

func TestDeleteJob(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	j := newJob("delete-me")
	if err := s.CreateJob(ctx, j); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if err := s.DeleteJob(ctx, j.ID); err != nil {
		t.Fatalf("DeleteJob: %v", err)
	}

	_, err := s.GetJob(ctx, j.ID)
	if err == nil {
		t.Fatal("expected error after delete, got nil")
	}
}

// --- Run tests ---

func newTestRun(jobID uuid.UUID) job.Run {
	return job.NewRun(jobID, "oc-session-abc")
}

func TestCreateAndGetRun(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	j := newJob("run-parent")
	if err := s.CreateJob(ctx, j); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	r := newTestRun(j.ID)
	if err := s.CreateRun(ctx, r); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	got, err := s.GetRun(ctx, r.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.ID != r.ID {
		t.Errorf("ID: got %v, want %v", got.ID, r.ID)
	}
	if got.JobID != j.ID {
		t.Errorf("JobID: got %v, want %v", got.JobID, j.ID)
	}
	if got.OCSessionID != "oc-session-abc" {
		t.Errorf("OCSessionID: got %q, want %q", got.OCSessionID, "oc-session-abc")
	}
	if got.Status != job.RunStatusRunning {
		t.Errorf("Status: got %v, want %v", got.Status, job.RunStatusRunning)
	}
	if got.EndedAt != nil {
		t.Errorf("EndedAt: got %v, want nil", got.EndedAt)
	}
}

func TestListRunsOrderedByStartedAtDesc(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	j := newJob("list-runs-parent")
	if err := s.CreateJob(ctx, j); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	// Create 3 runs with increasing start times.
	var ids []uuid.UUID
	for i := 0; i < 3; i++ {
		r := job.Run{
			ID:          uuid.New(),
			JobID:       j.ID,
			StartedAt:   time.Now().UTC().Add(time.Duration(i) * time.Second),
			OCSessionID: fmt.Sprintf("session-%d", i),
			Status:      job.RunStatusRunning,
		}
		if err := s.CreateRun(ctx, r); err != nil {
			t.Fatalf("CreateRun %d: %v", i, err)
		}
		ids = append(ids, r.ID)
	}

	runs, err := s.ListRuns(ctx, j.ID)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 3 {
		t.Fatalf("len: got %d, want 3", len(runs))
	}
	// Should be DESC — newest first.
	if runs[0].ID != ids[2] {
		t.Errorf("first run ID: got %v, want %v (newest)", runs[0].ID, ids[2])
	}
	if runs[2].ID != ids[0] {
		t.Errorf("last run ID: got %v, want %v (oldest)", runs[2].ID, ids[0])
	}
}

func TestLatestRun(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	j := newJob("latest-run-parent")
	if err := s.CreateJob(ctx, j); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	r1 := job.Run{
		ID:          uuid.New(),
		JobID:       j.ID,
		StartedAt:   time.Now().UTC(),
		OCSessionID: "first",
		Status:      job.RunStatusDone,
	}
	time.Sleep(2 * time.Millisecond)
	r2 := job.Run{
		ID:          uuid.New(),
		JobID:       j.ID,
		StartedAt:   time.Now().UTC(),
		OCSessionID: "second",
		Status:      job.RunStatusDone,
	}
	for _, r := range []job.Run{r1, r2} {
		if err := s.CreateRun(ctx, r); err != nil {
			t.Fatalf("CreateRun: %v", err)
		}
	}

	latest, err := s.LatestRun(ctx, j.ID)
	if err != nil {
		t.Fatalf("LatestRun: %v", err)
	}
	if latest.ID != r2.ID {
		t.Errorf("LatestRun ID: got %v, want %v", latest.ID, r2.ID)
	}
}

func TestLatestRunNoRuns(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	j := newJob("empty-run-parent")
	if err := s.CreateJob(ctx, j); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	_, err := s.LatestRun(ctx, j.ID)
	if err == nil {
		t.Fatal("expected error for no runs, got nil")
	}
}

func TestUpdateRun(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	j := newJob("update-run-parent")
	if err := s.CreateJob(ctx, j); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	r := newTestRun(j.ID)
	if err := s.CreateRun(ctx, r); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	now := time.Now().UTC()
	r.EndedAt = &now
	r.Status = job.RunStatusDone
	r.Output = "PAIGE_STATUS: done"
	r.AgentDone = true
	if err := s.UpdateRun(ctx, r); err != nil {
		t.Fatalf("UpdateRun: %v", err)
	}

	got, err := s.GetRun(ctx, r.ID)
	if err != nil {
		t.Fatalf("GetRun after update: %v", err)
	}
	if got.Status != job.RunStatusDone {
		t.Errorf("Status: got %v, want done", got.Status)
	}
	if !got.AgentDone {
		t.Error("AgentDone: got false, want true")
	}
	if got.Output != "PAIGE_STATUS: done" {
		t.Errorf("Output: got %q", got.Output)
	}
	if got.EndedAt == nil {
		t.Error("EndedAt: got nil, want non-nil")
	}
}
