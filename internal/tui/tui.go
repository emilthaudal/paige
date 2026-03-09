// Package tui implements the Paige terminal user interface using Bubble Tea.
package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/emtb/paige/internal/daemon"
	"github.com/emtb/paige/internal/job"
	"github.com/emtb/paige/internal/store"
)

// view is an enum for which screen is currently displayed.
type view int

const (
	viewJobList view = iota
	viewJobDetail
	viewAddJob
)

// navigateToDetailMsg is sent when the user selects a job from the list.
type navigateToDetailMsg struct{ job job.Job }

// navigateToListMsg is sent when the user navigates back from the detail view.
// If refresh is true, the list should reload jobs.
type navigateToListMsg struct{ refresh bool }

// Model is the root Bubble Tea model. It owns the active view and shared state.
type Model struct {
	daemon  *daemon.Daemon
	store   store.Store
	current view
	width   int
	height  int

	// child models (initialized lazily)
	jobList   *JobListModel
	jobDetail *JobDetailModel
}

// NewModel creates the root TUI model.
func NewModel(d *daemon.Daemon, st store.Store) Model {
	return Model{
		daemon:    d,
		store:     st,
		current:   viewJobList,
		jobList:   NewJobListModel(d, st),
		jobDetail: NewJobDetailModel(d, st, job.Job{}),
	}
}

// Init starts any initial commands.
func (m Model) Init() tea.Cmd {
	return m.jobList.Init()
}

// Update handles messages and delegates to the active child model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.jobList.SetSize(msg.Width, msg.Height)
		m.jobDetail.SetSize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		}

	case navigateToDetailMsg:
		m.jobDetail = NewJobDetailModel(m.daemon, m.store, msg.job)
		m.jobDetail.SetSize(m.width, m.height)
		m.current = viewJobDetail
		return m, m.jobDetail.Init()

	case navigateToListMsg:
		m.current = viewJobList
		if msg.refresh {
			m.jobList.loading = true
			return m, m.jobList.loadJobs()
		}
		return m, nil
	}

	switch m.current {
	case viewJobList:
		updated, cmd := m.jobList.Update(msg)
		m.jobList = updated.(*JobListModel)
		return m, cmd
	case viewJobDetail:
		updated, cmd := m.jobDetail.Update(msg)
		m.jobDetail = updated.(*JobDetailModel)
		return m, cmd
	}

	return m, nil
}

// View renders the current screen.
func (m Model) View() string {
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		Render("Paige")

	switch m.current {
	case viewJobList:
		return fmt.Sprintf("%s\n\n%s", header, m.jobList.View())
	case viewJobDetail:
		return fmt.Sprintf("%s\n\n%s", header, m.jobDetail.View())
	default:
		return header + "\n\n(coming soon)"
	}
}

// Run starts the Bubble Tea program.
func Run(d *daemon.Daemon, st store.Store) error {
	m := NewModel(d, st)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
