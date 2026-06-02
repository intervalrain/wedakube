package tui

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/intervalrain/wedakube/internal/cluster"
	"github.com/intervalrain/wedakube/internal/model"
)

const refreshInterval = 3 * time.Second

type servicesMsg []model.Service
type errMsg struct{ err error }
type tickMsg time.Time

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("63")).Padding(0, 1)
	statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
	footerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).MarginTop(1)
)

type Model struct {
	kubectl  *cluster.Kubectl
	table    table.Model
	err      error
	loading  bool
	lastSync time.Time
}

func New(kubectl *cluster.Kubectl) Model {
	columns := []table.Column{
		{Title: "NAME", Width: 26},
		{Title: "READY", Width: 7},
		{Title: "UP-TO-DATE", Width: 11},
		{Title: "IMAGE", Width: 44},
	}
	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(16),
	)
	st := table.DefaultStyles()
	st.Header = st.Header.Bold(true).BorderBottom(true).BorderForeground(lipgloss.Color("240"))
	st.Selected = st.Selected.Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("63"))
	t.SetStyles(st)

	return Model{kubectl: kubectl, table: t, loading: true}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.fetch(), tick())
}

func tick() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// fetch 是一個 command：在背景 goroutine 撈 deployments，回傳成 Msg。
func (m Model) fetch() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		svcs, err := m.kubectl.Deployments(ctx)
		if err != nil {
			return errMsg{err}
		}
		return servicesMsg(svcs)
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "r":
			m.loading = true
			return m, m.fetch()
		}
	case servicesMsg:
		m.loading = false
		m.err = nil
		m.lastSync = time.Now()
		m.setRows(msg)
		return m, nil
	case errMsg:
		m.loading = false
		m.err = msg.err
		return m, nil
	case tickMsg:
		return m, tea.Batch(m.fetch(), tick())
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m *Model) setRows(svcs []model.Service) {
	rows := make([]table.Row, 0, len(svcs))
	for _, s := range svcs {
		rows = append(rows, table.Row{s.Name, s.Ready, strconv.Itoa(s.UpToDate), s.ShortImage()})
	}
	m.table.SetRows(rows)
}

func (m Model) View() string {
	header := titleStyle.Render("WEDA k3s console")

	var status string
	switch {
	case m.err != nil:
		status = errStyle.Render("error: " + m.err.Error())
	case m.loading && len(m.table.Rows()) == 0:
		status = statusStyle.Render("loading...")
	default:
		status = statusStyle.Render(fmt.Sprintf("ns=%s  services=%d  synced %s",
			m.kubectl.Namespace(), len(m.table.Rows()), m.lastSync.Format("15:04:05")))
	}

	footer := footerStyle.Render("↑/↓ navigate · r refresh · q quit")

	return lipgloss.JoinVertical(lipgloss.Left, header, m.table.View(), status, footer)
}
