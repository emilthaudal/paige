package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/emtb/paige/internal/daemon"
	"github.com/emtb/paige/internal/job"
	"github.com/emtb/paige/internal/store"
)

// JobDetailModel shows the details and run history for a single job.
// This is a stub — full implementation coming.
type JobDetailModel struct {
	daemon *daemon.Daemon
	store  store.Store
	job    job.Job
}

// NewJobDetailModel creates a detail model for a given job.
func NewJobDetailModel(d *daemon.Daemon, st store.Store, j job.Job) *JobDetailModel {
	return &JobDetailModel{daemon: d, store: st, job: j}
}

func (m *JobDetailModel) Init() tea.Cmd { return nil }

func (m *JobDetailModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return m, nil
}

func (m *JobDetailModel) View() string {
	return "Job detail view — coming soon\n\nJob: " + m.job.Name
}
