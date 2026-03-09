package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/emtb/paige/internal/daemon"
	"github.com/emtb/paige/internal/job"
	"github.com/emtb/paige/internal/store"
)

// jobItem wraps a job.Job to implement list.Item.
type jobItem struct {
	j job.Job
}

func (i jobItem) Title() string {
	return fmt.Sprintf("%s  %s", stateIcon(i.j.State), i.j.Name)
}

func (i jobItem) Description() string {
	return fmt.Sprintf("%s  %s  updated %s",
		i.j.Repo, i.j.Schedule, humanTime(i.j.UpdatedAt))
}

func (i jobItem) FilterValue() string { return i.j.Name }

// cancelledMsg is sent when a CancelJob call completes (err may be nil).
type cancelledMsg struct{ err error }

// JobListModel is the job list screen.
type JobListModel struct {
	daemon     *daemon.Daemon
	store      store.Store
	list       list.Model
	loading    bool
	err        error
	confirming bool   // true while waiting for y/n to cancel a job
	cancelID   string // name of the job being confirmed for cancellation
}

type jobsLoadedMsg struct{ jobs []job.Job }
type jobsErrMsg struct{ err error }

// keyBindings holds the extra keys shown in the help footer.
var keyBindings = struct {
	cancel  key.Binding
	refresh key.Binding
}{
	cancel: key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "cancel job"),
	),
	refresh: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "refresh"),
	),
}

// NewJobListModel creates a job list model.
func NewJobListModel(d *daemon.Daemon, st store.Store) *JobListModel {
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(lipgloss.Color("205")).
		BorderForeground(lipgloss.Color("205"))
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.
		Foreground(lipgloss.Color("241"))

	l := list.New(nil, delegate, 0, 0)
	l.Title = "Jobs"
	l.SetShowHelp(true)
	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{keyBindings.cancel, keyBindings.refresh}
	}
	l.AdditionalFullHelpKeys = func() []key.Binding {
		return []key.Binding{keyBindings.cancel, keyBindings.refresh}
	}

	return &JobListModel{
		daemon: d,
		store:  st,
		list:   l,
	}
}

func (m *JobListModel) SetSize(w, h int) {
	m.list.SetSize(w, h-2)
}

func (m *JobListModel) Init() tea.Cmd {
	return m.loadJobs()
}

func (m *JobListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case jobsLoadedMsg:
		m.loading = false
		items := make([]list.Item, len(msg.jobs))
		for i, j := range msg.jobs {
			items[i] = jobItem{j}
		}
		return m, m.list.SetItems(items)

	case jobsErrMsg:
		m.loading = false
		m.err = msg.err
		return m, nil

	case cancelledMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		// Refresh the list after a successful cancel.
		m.loading = true
		return m, m.loadJobs()

	case tea.KeyMsg:
		// Confirmation prompt intercepts all keys.
		if m.confirming {
			switch msg.String() {
			case "y", "Y":
				m.confirming = false
				return m, m.cancelFocusedJob()
			default:
				m.confirming = false
				return m, nil
			}
		}

		switch msg.String() {
		case "r":
			m.loading = true
			return m, m.loadJobs()
		case "c":
			if item, ok := m.list.SelectedItem().(jobItem); ok {
				j := item.j
				// Only allow cancelling non-terminal jobs.
				if j.State != job.StateCompleted && j.State != job.StateCancelled {
					m.confirming = true
					m.cancelID = j.Name
					return m, nil
				}
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *JobListModel) View() string {
	if m.loading {
		return "Loading jobs..."
	}
	if m.err != nil {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("9")).
			Render("Error: " + m.err.Error())
	}
	if m.confirming {
		prompt := lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")).
			Bold(true).
			Render(fmt.Sprintf("Cancel job %q? (y/N) ", m.cancelID))
		return m.list.View() + "\n\n" + prompt
	}
	return m.list.View()
}

func (m *JobListModel) loadJobs() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		jobs, err := m.store.ListJobs(ctx, store.JobFilter{})
		if err != nil {
			return jobsErrMsg{err}
		}
		return jobsLoadedMsg{jobs}
	}
}

func (m *JobListModel) cancelFocusedJob() tea.Cmd {
	item, ok := m.list.SelectedItem().(jobItem)
	if !ok {
		return nil
	}
	id := item.j.ID
	d := m.daemon
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return cancelledMsg{err: d.CancelJob(ctx, id)}
	}
}

// --- helpers ---

func stateIcon(s job.State) string {
	switch s {
	case job.StateActive:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render("●")
	case job.StateRunning:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render("◌")
	case job.StatePending:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Render("◉")
	case job.StateCompleted:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("✓")
	case job.StateCancelled:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("✕")
	case job.StatePaused:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render("⏸")
	default:
		return "?"
	}
}

func humanTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
