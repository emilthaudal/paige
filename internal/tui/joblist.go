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

// filterTab represents a state filter tab on the job list.
type filterTab int

const (
	tabAll filterTab = iota
	tabActive
	tabRunning
	tabPending
	tabCompleted
	tabCancelled
	tabPaused
	tabCount // sentinel — total number of tabs
)

// tabLabel returns the display label for a filter tab.
func tabLabel(t filterTab) string {
	switch t {
	case tabAll:
		return "All"
	case tabActive:
		return "Active"
	case tabRunning:
		return "Running"
	case tabPending:
		return "Pending"
	case tabCompleted:
		return "Completed"
	case tabCancelled:
		return "Cancelled"
	case tabPaused:
		return "Paused"
	default:
		return "?"
	}
}

// tabState returns the job.State filter for a tab (nil = all).
func tabState(t filterTab) []job.State {
	switch t {
	case tabActive:
		return []job.State{job.StateActive}
	case tabRunning:
		return []job.State{job.StateRunning}
	case tabPending:
		return []job.State{job.StatePending}
	case tabCompleted:
		return []job.State{job.StateCompleted}
	case tabCancelled:
		return []job.State{job.StateCancelled}
	case tabPaused:
		return []job.State{job.StatePaused}
	default:
		return nil
	}
}

// tabBar renders the tab bar using lipgloss.
func tabBar(active filterTab) string {
	activeStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		Underline(true)
	inactiveStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241"))

	tabs := make([]string, tabCount)
	for i := filterTab(0); i < tabCount; i++ {
		label := tabLabel(i)
		if i == active {
			tabs[i] = activeStyle.Render(label)
		} else {
			tabs[i] = inactiveStyle.Render(label)
		}
	}

	sep := lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Render("  │  ")
	result := ""
	for i, t := range tabs {
		if i > 0 {
			result += sep
		}
		result += t
	}
	return result
}

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
	activeTab  filterTab
}

type jobsLoadedMsg struct{ jobs []job.Job }
type jobsErrMsg struct{ err error }

// keyBindings holds the extra keys shown in the help footer.
var keyBindings = struct {
	cancel    key.Binding
	refresh   key.Binding
	tabNext   key.Binding
	tabPrev   key.Binding
	selectJob key.Binding
}{
	cancel: key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "cancel job"),
	),
	refresh: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "refresh"),
	),
	tabNext: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "next filter"),
	),
	tabPrev: key.NewBinding(
		key.WithKeys("shift+tab"),
		key.WithHelp("shift+tab", "prev filter"),
	),
	selectJob: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "view detail"),
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
		return []key.Binding{keyBindings.selectJob, keyBindings.cancel, keyBindings.refresh, keyBindings.tabNext}
	}
	l.AdditionalFullHelpKeys = func() []key.Binding {
		return []key.Binding{keyBindings.selectJob, keyBindings.cancel, keyBindings.refresh, keyBindings.tabNext, keyBindings.tabPrev}
	}

	return &JobListModel{
		daemon:    d,
		store:     st,
		list:      l,
		activeTab: tabAll,
	}
}

func (m *JobListModel) SetSize(w, h int) {
	// Reserve 2 lines for the tab bar (1 bar + 1 blank line).
	m.list.SetSize(w, h-4)
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
		case "tab":
			m.activeTab = (m.activeTab + 1) % tabCount
			m.loading = true
			return m, m.loadJobs()
		case "shift+tab":
			m.activeTab = (m.activeTab - 1 + tabCount) % tabCount
			m.loading = true
			return m, m.loadJobs()
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
		case "enter":
			if item, ok := m.list.SelectedItem().(jobItem); ok {
				return m, func() tea.Msg {
					return navigateToDetailMsg{job: item.j}
				}
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *JobListModel) View() string {
	bar := tabBar(m.activeTab)

	if m.loading {
		return bar + "\n\nLoading jobs..."
	}
	if m.err != nil {
		return bar + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("9")).
			Render("Error: "+m.err.Error())
	}
	if m.confirming {
		prompt := lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")).
			Bold(true).
			Render(fmt.Sprintf("Cancel job %q? (y/N) ", m.cancelID))
		return bar + "\n\n" + m.list.View() + "\n\n" + prompt
	}
	return bar + "\n\n" + m.list.View()
}

func (m *JobListModel) loadJobs() tea.Cmd {
	states := tabState(m.activeTab)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		jobs, err := m.store.ListJobs(ctx, store.JobFilter{States: states})
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
