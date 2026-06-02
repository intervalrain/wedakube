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
	"github.com/intervalrain/wedakube/internal/config"
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

// ServiceList 是 L2：某 host 內的服務列表。
type ServiceList struct {
	kubectl  *cluster.Kubectl
	host     string
	store    *config.Store
	targets  map[string]config.Target // service name -> 部署目標
	table    table.Model
	services []model.Service
	err      error
	loading  bool
	lastSync time.Time
}

func NewServiceList(kubectl *cluster.Kubectl, host string, store *config.Store) ServiceList {
	targets := map[string]config.Target{}
	if ts, err := store.TargetsForHost(host); err == nil {
		for _, t := range ts {
			targets[t.Service] = t
		}
	}
	columns := []table.Column{
		{Title: "NAME", Width: 24},
		{Title: "READY", Width: 6},
		{Title: "UP-TO-DATE", Width: 11},
		{Title: "AGE", Width: 6},
		{Title: "IMAGE", Width: 38},
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

	return ServiceList{kubectl: kubectl, host: host, store: store, targets: targets, table: t, loading: true}
}

func (m ServiceList) Init() tea.Cmd {
	return tea.Batch(m.fetch(), tick())
}

func tick() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m ServiceList) fetch() tea.Cmd {
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

func (m ServiceList) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc":
			return m, pop()
		case "r":
			m.loading = true
			return m, m.fetch()
		case "enter":
			i := m.table.Cursor()
			if i >= 0 && i < len(m.services) {
				svc := m.services[i]
				var tgt *config.Target
				if t, ok := m.targets[svc.Name]; ok {
					tgt = &t
				}
				return m, push(NewServiceDetail(m.kubectl, m.host, m.store, svc, tgt))
			}
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

func (m *ServiceList) setRows(discovered []model.Service) {
	seen := map[string]bool{}
	merged := make([]model.Service, 0, len(discovered)+len(m.targets))
	for _, s := range discovered {
		seen[s.Name] = true
		merged = append(merged, s)
	}
	// 加入有設定 target、但 cluster 還沒部署的服務（新 service）。
	for name, t := range m.targets {
		if !seen[name] {
			merged = append(merged, model.Service{
				Name: t.Service, Ready: "–", Age: "–", Image: "(not deployed)",
			})
		}
	}

	m.services = merged
	rows := make([]table.Row, 0, len(merged))
	for _, s := range merged {
		name := s.Name
		if _, ok := m.targets[s.Name]; ok {
			name = "◆ " + s.Name
		}
		rows = append(rows, table.Row{name, s.Ready, strconv.Itoa(s.UpToDate), s.Age, s.ShortImage()})
	}
	m.table.SetRows(rows)
}

func (m ServiceList) View() string {
	header := titleStyle.Render(m.host + " · services")

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

	footer := footerStyle.Render("↑/↓ navigate · enter open · r refresh · esc back · ctrl+c quit")

	return lipgloss.JoinVertical(lipgloss.Left, header, m.table.View(), status, footer)
}
