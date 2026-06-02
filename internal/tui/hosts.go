package tui

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/intervalrain/wedakube/internal/cluster"
	"github.com/intervalrain/wedakube/internal/config"
)

type connectedMsg struct {
	kc   *cluster.Kubectl
	host string
}

// HostsScreen 是 L1：工具管理的主機清單。
type HostsScreen struct {
	store   *config.Store
	table   table.Model
	hosts   []config.Host
	err     error
	connect string // 正在連線中的 host name（顯示用）
}

func NewHosts(store *config.Store) HostsScreen {
	columns := []table.Column{
		{Title: "NAME", Width: 24},
		{Title: "HOST", Width: 18},
		{Title: "USER", Width: 10},
		{Title: "NS", Width: 24},
	}
	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(14),
	)
	st := table.DefaultStyles()
	st.Header = st.Header.Bold(true).BorderBottom(true).BorderForeground(lipgloss.Color("240"))
	st.Selected = st.Selected.Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("63"))
	t.SetStyles(st)

	h := HostsScreen{store: store, table: t}
	return h.reload()
}

func (m HostsScreen) reload() HostsScreen {
	hosts, err := m.store.ListHosts()
	m.err = err
	m.hosts = hosts

	rows := make([]table.Row, 0, len(hosts))
	for _, h := range hosts {
		ns := h.Namespace
		if ns == "" {
			ns = "(auto -weda)"
		}
		dest := h.Dest()
		rows = append(rows, table.Row{h.Name, dest, h.User, ns})
	}
	m.table.SetRows(rows)
	return m
}

func (m HostsScreen) Init() tea.Cmd { return nil }

func (m HostsScreen) selected() (config.Host, bool) {
	i := m.table.Cursor()
	if i < 0 || i >= len(m.hosts) {
		return config.Host{}, false
	}
	return m.hosts[i], true
}

func (m HostsScreen) connectCmd(h config.Host) tea.Cmd {
	return func() tea.Msg {
		ssh := cluster.NewSSH(h)
		ns := h.Namespace
		if ns == "" {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			resolved, err := cluster.ResolveWedaNamespace(ctx, ssh)
			if err != nil {
				return errMsg{err}
			}
			ns = resolved
		}
		return connectedMsg{kc: cluster.NewKubectl(ssh, ns), host: h.Name}
	}
}

func (m HostsScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q":
			return m, tea.Quit
		case "r":
			return m.reload(), nil
		case "d":
			if h, ok := m.selected(); ok {
				m.store.DeleteHost(h.Name)
				return m.reload(), nil
			}
		case "enter":
			if h, ok := m.selected(); ok {
				m.connect = h.Name
				m.err = nil
				return m, m.connectCmd(h)
			}
		}
	case connectedMsg:
		m.connect = ""
		return m, push(NewServiceList(msg.kc, msg.host, m.store))
	case errMsg:
		m.connect = ""
		m.err = msg.err
		return m, nil
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m HostsScreen) View() string {
	header := titleStyle.Render("wedakube · hosts")

	var status string
	switch {
	case m.err != nil:
		status = errStyle.Render("error: " + m.err.Error())
	case m.connect != "":
		status = statusStyle.Render("connecting to " + m.connect + " ...")
	case len(m.hosts) == 0:
		status = statusStyle.Render("no hosts — press n to add (form coming in M3.2)")
	default:
		status = statusStyle.Render(time.Now().Format("15:04:05"))
	}

	footer := footerStyle.Render("↑/↓ navigate · enter connect · n new · e edit · d delete · i info · r refresh · q quit")

	return lipgloss.JoinVertical(lipgloss.Left, header, m.table.View(), status, footer)
}
