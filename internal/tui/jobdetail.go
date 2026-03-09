package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/emtb/paige/internal/daemon"
	"github.com/emtb/paige/internal/job"
	"github.com/emtb/paige/internal/store"
)

// jobDetailLoadedMsg is sent when the job and its runs have been fetched.
type jobDetailLoadedMsg struct {
	j    job.Job
	runs []job.Run
}

// jobDetailErrMsg is sent when loading job detail data fails.
type jobDetailErrMsg struct{ err error }

// confirmedMsg is sent when a ConfirmJob call completes.
type confirmedMsg struct{ err error }

// confirmAction distinguishes confirm-done from cancel in the detail view.
type confirmAction int

const (
	confirmActionConfirm confirmAction = iota
	confirmActionCancel
)

// detailKeyBindings holds the keys for the detail view help footer.
var detailKeyBindings = struct {
	back    key.Binding
	refresh key.Binding
	confirm key.Binding
	cancel  key.Binding
}{
	back: key.NewBinding(
		key.WithKeys("esc", "b"),
		key.WithHelp("esc/b", "back"),
	),
	refresh: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "refresh"),
	),
	confirm: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "confirm done"),
	),
	cancel: key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "cancel job"),
	),
}

// JobDetailModel shows the details and run history for a single job.
type JobDetailModel struct {
	daemon        *daemon.Daemon
	store         store.Store
	job           job.Job
	runs          []job.Run
	viewport      viewport.Model
	loading       bool
	err           error
	width         int
	height        int
	confirming    bool
	confirmTarget confirmAction
}

// NewJobDetailModel creates a detail model for the given job.
func NewJobDetailModel(d *daemon.Daemon, st store.Store, j job.Job) *JobDetailModel {
	vp := viewport.New(0, 0)
	return &JobDetailModel{
		daemon:   d,
		store:    st,
		job:      j,
		viewport: vp,
		loading:  true,
	}
}

func (m *JobDetailModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	// metadata panel is ~8 lines + 2 separators; footer is 2 lines
	vpHeight := h - 12
	if vpHeight < 4 {
		vpHeight = 4
	}
	m.viewport.Width = w
	m.viewport.Height = vpHeight
}

func (m *JobDetailModel) Init() tea.Cmd {
	return m.loadDetail()
}

func (m *JobDetailModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case jobDetailLoadedMsg:
		m.loading = false
		m.err = nil
		m.job = msg.j
		m.runs = msg.runs
		m.viewport.SetContent(m.renderRuns())
		return m, nil

	case jobDetailErrMsg:
		m.loading = false
		m.err = msg.err
		return m, nil

	case confirmedMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		return m, func() tea.Msg { return navigateToListMsg{refresh: true} }

	case cancelledMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		return m, func() tea.Msg { return navigateToListMsg{refresh: true} }

	case tea.KeyMsg:
		if m.confirming {
			switch msg.String() {
			case "y", "Y":
				m.confirming = false
				if m.confirmTarget == confirmActionConfirm {
					return m, m.confirmJob()
				}
				return m, m.cancelJob()
			default:
				m.confirming = false
				return m, nil
			}
		}

		switch msg.String() {
		case "esc", "b":
			return m, func() tea.Msg { return navigateToListMsg{} }
		case "r":
			m.loading = true
			return m, m.loadDetail()
		case "enter":
			if m.job.State == job.StatePending {
				m.confirming = true
				m.confirmTarget = confirmActionConfirm
				return m, nil
			}
		case "c":
			if m.job.State != job.StateCompleted && m.job.State != job.StateCancelled {
				m.confirming = true
				m.confirmTarget = confirmActionCancel
				return m, nil
			}
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m *JobDetailModel) View() string {
	if m.loading {
		return "Loading job detail...\n\nesc/b  back"
	}
	if m.err != nil {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("9")).
			Render("Error: "+m.err.Error()) +
			"\n\nr  retry    esc/b  back"
	}

	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("241"))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))

	field := func(label, value string) string {
		return labelStyle.Render(fmt.Sprintf("%-12s", label)) + "  " + valueStyle.Render(value)
	}

	meta := strings.Join([]string{
		field("Name", m.job.Name),
		field("Repo", m.job.Repo),
		field("Schedule", m.job.Schedule),
		field("State", fmt.Sprintf("%s  %s", stateIcon(m.job.State), string(m.job.State))),
		field("Created", m.job.CreatedAt.Local().Format("2006-01-02 15:04:05")),
		field("Updated", humanTime(m.job.UpdatedAt)),
	}, "\n")

	divider := lipgloss.NewStyle().Foreground(lipgloss.Color("238")).
		Render(strings.Repeat("─", m.width))

	runsHeader := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).
		Render(fmt.Sprintf("Run History  (%d)", len(m.runs)))

	body := meta + "\n\n" + divider + "\n" + runsHeader + "\n\n" + m.viewport.View()

	// Footer: confirmation prompt or contextual keybind hints.
	var footer string
	if m.confirming {
		if m.confirmTarget == confirmActionConfirm {
			footer = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10")).
				Render("Confirm job done? (y/N) ")
		} else {
			footer = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11")).
				Render(fmt.Sprintf("Cancel job %q? (y/N) ", m.job.Name))
		}
	} else {
		hints := []string{"esc/b  back", "r  refresh"}
		if m.job.State == job.StatePending {
			hints = append(hints, "enter  confirm done", "c  cancel")
		} else if m.job.State != job.StateCompleted && m.job.State != job.StateCancelled {
			hints = append(hints, "c  cancel")
		}
		footer = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).
			Render(strings.Join(hints, "    "))
	}

	return body + "\n\n" + divider + "\n" + footer
}

// renderRuns produces the viewport content for the run history.
func (m *JobDetailModel) renderRuns() string {
	if len(m.runs) == 0 {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("241")).
			Render("No runs yet.")
	}

	runStatusIcon := func(s job.RunStatus) string {
		switch s {
		case job.RunStatusDone:
			return lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render("✓")
		case job.RunStatusFailed:
			return lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("✕")
		default:
			return lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render("◌")
		}
	}

	agentDoneIcon := func(done bool) string {
		if done {
			return lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render("agent:done")
		}
		return lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("agent:not_done")
	}

	var sb strings.Builder
	for i, r := range m.runs {
		started := r.StartedAt.Local().Format("2006-01-02 15:04:05")
		duration := ""
		if r.EndedAt != nil {
			d := r.EndedAt.Sub(r.StartedAt).Round(time.Second)
			duration = fmt.Sprintf("  %s", d)
		}

		header := fmt.Sprintf("%s  %s%s  %s",
			runStatusIcon(r.Status),
			started,
			duration,
			agentDoneIcon(r.AgentDone),
		)
		sb.WriteString(header + "\n")

		if r.Output != "" {
			// Show up to 3 lines of output, truncated.
			lines := strings.Split(strings.TrimSpace(r.Output), "\n")
			preview := lines
			truncated := false
			if len(preview) > 3 {
				preview = preview[:3]
				truncated = true
			}
			for _, line := range preview {
				if len(line) > m.width-4 && m.width > 4 {
					line = line[:m.width-4] + "…"
				}
				sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("243")).
					Render("  "+line) + "\n")
			}
			if truncated {
				sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("238")).
					Render(fmt.Sprintf("  … (%d more lines)", len(lines)-3)) + "\n")
			}
		}

		if i < len(m.runs)-1 {
			sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("238")).
				Render("  ·····") + "\n")
		}
	}
	return sb.String()
}

func (m *JobDetailModel) loadDetail() tea.Cmd {
	id := m.job.ID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		j, err := m.store.GetJob(ctx, id)
		if err != nil {
			return jobDetailErrMsg{err}
		}
		runs, err := m.store.ListRuns(ctx, id)
		if err != nil {
			return jobDetailErrMsg{err}
		}
		return jobDetailLoadedMsg{j: j, runs: runs}
	}
}

func (m *JobDetailModel) confirmJob() tea.Cmd {
	id := m.job.ID
	d := m.daemon
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return confirmedMsg{err: d.ConfirmJob(ctx, id)}
	}
}

func (m *JobDetailModel) cancelJob() tea.Cmd {
	id := m.job.ID
	d := m.daemon
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return cancelledMsg{err: d.CancelJob(ctx, id)}
	}
}
