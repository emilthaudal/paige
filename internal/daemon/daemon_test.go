package daemon_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/emtb/paige/internal/daemon"
	"github.com/emtb/paige/internal/job"
	"github.com/emtb/paige/internal/opencode"
	"github.com/emtb/paige/internal/store"
)

// --- mock OCClient ---

type mockOCClient struct {
	mu              sync.Mutex
	createSessionFn func(ctx context.Context, title string) (opencode.Session, error)
	sendPromptFn    func(ctx context.Context, sessionID string, prompt string) (opencode.MessageResponse, error)
	deleteSessionFn func(ctx context.Context, sessionID string) error
}

func (m *mockOCClient) CreateSession(ctx context.Context, title string) (opencode.Session, error) {
	if m.createSessionFn != nil {
		return m.createSessionFn(ctx, title)
	}
	return opencode.Session{ID: "mock-session"}, nil
}

func (m *mockOCClient) SendPrompt(ctx context.Context, sessionID string, prompt string) (opencode.MessageResponse, error) {
	if m.sendPromptFn != nil {
		return m.sendPromptFn(ctx, sessionID, prompt)
	}
	return opencode.MessageResponse{
		Parts: []opencode.MessagePart{{Type: "text", Text: "PAIGE_STATUS: done"}},
	}, nil
}

func (m *mockOCClient) DeleteSession(ctx context.Context, sessionID string) error {
	if m.deleteSessionFn != nil {
		return m.deleteSessionFn(ctx, sessionID)
	}
	return nil
}

// --- mock Store ---

type mockStore struct {
	mu   sync.Mutex
	jobs map[uuid.UUID]job.Job
	runs map[uuid.UUID]job.Run
}

func newMockStore() *mockStore {
	return &mockStore{
		jobs: make(map[uuid.UUID]job.Job),
		runs: make(map[uuid.UUID]job.Run),
	}
}

func (s *mockStore) CreateJob(_ context.Context, j job.Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[j.ID] = j
	return nil
}

func (s *mockStore) GetJob(_ context.Context, id uuid.UUID) (job.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[id]
	if !ok {
		return job.Job{}, errors.New("job not found")
	}
	return j, nil
}

func (s *mockStore) ListJobs(_ context.Context, _ store.JobFilter) ([]job.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []job.Job
	for _, j := range s.jobs {
		out = append(out, j)
	}
	return out, nil
}

func (s *mockStore) UpdateJob(_ context.Context, j job.Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[j.ID] = j
	return nil
}

func (s *mockStore) DeleteJob(_ context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.jobs, id)
	return nil
}

func (s *mockStore) CreateRun(_ context.Context, r job.Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs[r.ID] = r
	return nil
}

func (s *mockStore) GetRun(_ context.Context, id uuid.UUID) (job.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[id]
	if !ok {
		return job.Run{}, errors.New("run not found")
	}
	return r, nil
}

func (s *mockStore) ListRuns(_ context.Context, jobID uuid.UUID) ([]job.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []job.Run
	for _, r := range s.runs {
		if r.JobID == jobID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *mockStore) UpdateRun(_ context.Context, r job.Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs[r.ID] = r
	return nil
}

func (s *mockStore) LatestRun(_ context.Context, jobID uuid.UUID) (job.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var latest job.Run
	found := false
	for _, r := range s.runs {
		if r.JobID == jobID {
			if !found || r.StartedAt.After(latest.StartedAt) {
				latest = r
				found = true
			}
		}
	}
	if !found {
		return job.Run{}, errors.New("no runs found")
	}
	return latest, nil
}

func (s *mockStore) Close() error { return nil }

// --- helpers ---

func newTestDaemon(t *testing.T, st store.Store, oc opencode.OCClient) *daemon.Daemon {
	t.Helper()
	d, err := daemon.New(st, oc)
	if err != nil {
		t.Fatalf("daemon.New: %v", err)
	}
	return d
}

func createPendingJob(t *testing.T, s *mockStore) job.Job {
	t.Helper()
	j := job.NewJob("test-job", "github.com/test/repo", "do the thing", "*/5 * * * *")
	j.State = job.StatePending
	if err := s.CreateJob(context.Background(), j); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	return j
}

func createActiveJob(t *testing.T, s *mockStore) job.Job {
	t.Helper()
	j := job.NewJob("active-job", "github.com/test/repo", "do the thing", "*/5 * * * *")
	if err := s.CreateJob(context.Background(), j); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	return j
}

// --- ConfirmJob tests ---

func TestConfirmJob_HappyPath(t *testing.T) {
	st := newMockStore()
	d := newTestDaemon(t, st, &mockOCClient{})

	j := createPendingJob(t, st)
	ctx := context.Background()

	if err := d.ConfirmJob(ctx, j.ID); err != nil {
		t.Fatalf("ConfirmJob: %v", err)
	}

	got, err := st.GetJob(ctx, j.ID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if got.State != job.StateCompleted {
		t.Errorf("State after confirm: got %v, want %v", got.State, job.StateCompleted)
	}
}

func TestConfirmJob_ErrorWhenNotPending(t *testing.T) {
	st := newMockStore()
	d := newTestDaemon(t, st, &mockOCClient{})

	j := createActiveJob(t, st) // active, not pending
	ctx := context.Background()

	err := d.ConfirmJob(ctx, j.ID)
	if err == nil {
		t.Fatal("expected error confirming non-pending job, got nil")
	}
}

func TestConfirmJob_ErrorWhenNotFound(t *testing.T) {
	st := newMockStore()
	d := newTestDaemon(t, st, &mockOCClient{})

	err := d.ConfirmJob(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error for missing job, got nil")
	}
}

// --- CancelJob tests ---

func TestCancelJob_HappyPath(t *testing.T) {
	st := newMockStore()
	d := newTestDaemon(t, st, &mockOCClient{})

	j := createActiveJob(t, st)
	ctx := context.Background()

	if err := d.CancelJob(ctx, j.ID); err != nil {
		t.Fatalf("CancelJob: %v", err)
	}

	got, err := st.GetJob(ctx, j.ID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if got.State != job.StateCancelled {
		t.Errorf("State after cancel: got %v, want %v", got.State, job.StateCancelled)
	}
}

func TestCancelJob_ErrorWhenAlreadyTerminal(t *testing.T) {
	st := newMockStore()
	d := newTestDaemon(t, st, &mockOCClient{})
	ctx := context.Background()

	for _, terminalState := range []job.State{job.StateCompleted, job.StateCancelled} {
		j := job.NewJob("terminal-job", "github.com/test/repo", "do the thing", "*/5 * * * *")
		j.State = terminalState
		if err := st.CreateJob(ctx, j); err != nil {
			t.Fatalf("CreateJob: %v", err)
		}
		err := d.CancelJob(ctx, j.ID)
		if err == nil {
			t.Errorf("expected error cancelling %v job, got nil", terminalState)
		}
	}
}

func TestCancelPendingJob(t *testing.T) {
	st := newMockStore()
	d := newTestDaemon(t, st, &mockOCClient{})

	j := createPendingJob(t, st)
	ctx := context.Background()

	if err := d.CancelJob(ctx, j.ID); err != nil {
		t.Fatalf("CancelJob on pending: %v", err)
	}

	got, err := st.GetJob(ctx, j.ID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if got.State != job.StateCancelled {
		t.Errorf("State: got %v, want cancelled", got.State)
	}
}

// --- executeJob integration tests ---

func TestExecuteJob_AgentDone_TransitionsToPending(t *testing.T) {
	st := newMockStore()
	oc := &mockOCClient{
		sendPromptFn: func(_ context.Context, _ string, _ string) (opencode.MessageResponse, error) {
			return opencode.MessageResponse{
				Parts: []opencode.MessagePart{{Type: "text", Text: "All done!\nPAIGE_STATUS: done"}},
			}, nil
		},
	}
	d := newTestDaemon(t, st, oc)

	j := createActiveJob(t, st)
	d.TriggerJob(j.ID)

	got, err := st.GetJob(context.Background(), j.ID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if got.State != job.StatePending {
		t.Errorf("State after agent done: got %v, want pending", got.State)
	}
}

func TestExecuteJob_AgentNotDone_RemainsActive(t *testing.T) {
	st := newMockStore()
	oc := &mockOCClient{
		sendPromptFn: func(_ context.Context, _ string, _ string) (opencode.MessageResponse, error) {
			return opencode.MessageResponse{
				Parts: []opencode.MessagePart{{Type: "text", Text: "Still working...\nPAIGE_STATUS: not_done"}},
			}, nil
		},
	}
	d := newTestDaemon(t, st, oc)

	j := createActiveJob(t, st)
	d.TriggerJob(j.ID)

	got, err := st.GetJob(context.Background(), j.ID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if got.State != job.StateActive {
		t.Errorf("State after agent not_done: got %v, want active", got.State)
	}
}

// --- unit tests for package-level helpers ---

func TestParseAgentDone_Done(t *testing.T) {
	cases := []struct {
		output string
		want   bool
	}{
		{"PAIGE_STATUS: done", true},
		{"some output\nPAIGE_STATUS: done\nmore", true},
		{"PAIGE_STATUS: not_done", false},
		{"no marker at all", false},
		{"PAIGE_STATUS: done_extra", true}, // contains substring
		{"", false},
	}
	for _, tc := range cases {
		got := daemon.ParseAgentDone(tc.output)
		if got != tc.want {
			t.Errorf("ParseAgentDone(%q) = %v, want %v", tc.output, got, tc.want)
		}
	}
}

func TestBuildPrompt_ContainsRepoAndPromptAndSuffix(t *testing.T) {
	j := job.NewJob("my-job", "github.com/foo/bar", "check the tests", "*/5 * * * *")
	got := daemon.BuildPrompt(j)

	if !containsAll(got, "github.com/foo/bar", "check the tests", "PAIGE_STATUS") {
		t.Errorf("BuildPrompt output missing expected content:\n%s", got)
	}
}

func containsAll(s string, substrings ...string) bool {
	for _, sub := range substrings {
		found := false
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func TestRegisterJob_StoresJob(t *testing.T) {
	st := newMockStore()
	d := newTestDaemon(t, st, &mockOCClient{})

	j := job.NewJob("reg-job", "github.com/test/repo", "do something", "*/10 * * * *")
	ctx := context.Background()

	if err := d.RegisterJob(ctx, j); err != nil {
		t.Fatalf("RegisterJob: %v", err)
	}

	got, err := st.GetJob(ctx, j.ID)
	if err != nil {
		t.Fatalf("GetJob after register: %v", err)
	}
	if got.Name != "reg-job" {
		t.Errorf("Name: got %q, want %q", got.Name, "reg-job")
	}
}

// Ensure mockOCClient satisfies the interface at compile time.
var _ opencode.OCClient = (*mockOCClient)(nil)

// Ensure mockStore satisfies the Store interface at compile time.
var _ store.Store = (*mockStore)(nil)

// Ensure time package is used (for future test expansion).
var _ = time.Second
