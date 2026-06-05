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
	helm config.HelmParams // 第一次連線時自動偵測，由 HostsScreen 存回 Host
}

// helmRefreshedMsg：R 鍵重新探測完，把新的 helm 參數寫回 host。
type helmRefreshedMsg struct {
	host string
	helm config.HelmParams
}

// hostsReloadMsg：n / e 表單存完 pop 回來時觸發 reload。
type hostsReloadMsg struct{}

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

func (m HostsScreen) Init() tea.Cmd {
	// pop 回 L1 時觸發 reload，這樣 n/e 表單存完馬上看得到
	return func() tea.Msg { return hostsReloadMsg{} }
}

func (m HostsScreen) selected() (config.Host, bool) {
	i := m.table.Cursor()
	if i < 0 || i >= len(m.hosts) {
		return config.Host{}, false
	}
	return m.hosts[i], true
}

// refreshHelmCmd 重跑 ResolveWedaNamespace + SampleHelmParams，把整包 helm 參數刷新。
// 用在 tenant 換掉、apiKey 輪替時，免得手編 state.json。
func (m HostsScreen) refreshHelmCmd(h config.Host) tea.Cmd {
	return func() tea.Msg {
		ssh := cluster.NewSSH(h)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		ns, err := cluster.ResolveWedaNamespace(ctx, ssh)
		if err != nil {
			return errMsg{err}
		}
		hp, err := cluster.NewKubectl(ssh, ns).SampleHelmParams(ctx)
		if err != nil {
			return errMsg{err}
		}
		return helmRefreshedMsg{host: h.Name, helm: hp}
	}
}

func (m HostsScreen) connectCmd(h config.Host) tea.Cmd {
	return func() tea.Msg {
		ssh := cluster.NewSSH(h)
		ns := h.DerivedNamespace()
		if ns == "" {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			resolved, err := cluster.ResolveWedaNamespace(ctx, ssh)
			if err != nil {
				return errMsg{err}
			}
			ns = resolved
		}
		kc := cluster.NewKubectl(ssh, ns)

		// 第一次連線：自動偵測 helm 參數（tenant/srp/eco/registry…）
		helm := h.Helm
		if helm.TenantID == "" {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if p, err := kc.SampleHelmParams(ctx); err == nil {
				helm = p
			}
		}
		return connectedMsg{kc: kc, host: h.Name, helm: helm}
	}
}

func (m HostsScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// 留 6 行給 header / status / footer / 邊框
		h := msg.Height - 6
		if h < 5 {
			h = 5
		}
		m.table.SetHeight(h)
		return m, nil
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
		case "S":
			return m, push(NewSetup(m.store))
		case "n":
			return m, push(NewHostForm(m.store, nil))
		case "e":
			if h, ok := m.selected(); ok {
				return m, push(NewHostForm(m.store, &h))
			}
		case "R":
			if h, ok := m.selected(); ok {
				m.connect = "refreshing helm params for " + h.Name
				m.err = nil
				return m, m.refreshHelmCmd(h)
			}
		case "enter":
			if h, ok := m.selected(); ok {
				m.connect = h.Name
				m.err = nil
				return m, m.connectCmd(h)
			}
		}
	case hostsReloadMsg:
		return m.reload(), nil
	case helmRefreshedMsg:
		m.connect = ""
		if h, ok, _ := m.store.GetHost(msg.host); ok {
			h.Helm = msg.helm
			m.store.PutHost(h)
			m = m.reload()
		}
		return m, nil
	case connectedMsg:
		m.connect = ""
		// 自動偵測到新的 helm 參數就持久化
		if h, ok, _ := m.store.GetHost(msg.host); ok && h.Helm.TenantID == "" && msg.helm.TenantID != "" {
			h.Helm = msg.helm
			m.store.PutHost(h)
			m = m.reload()
		}
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

	footer := footerStyle.Render("↑/↓ · enter connect · n new · e edit · d delete · S setup · r refresh · R refresh-helm · q quit")

	return lipgloss.JoinVertical(lipgloss.Left, header, m.table.View(), status, footer)
}
