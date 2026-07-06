package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/intervalrain/wedakube/internal/cluster"
	"github.com/intervalrain/wedakube/internal/config"
	"github.com/intervalrain/wedakube/internal/eco"
)

// ---------- EcoAppsScreen：L2 按 E，列出 ECO 裡所有 registered app ----------

type ecoAppsMsg struct {
	apps []eco.App
	err  error
}

type EcoAppsScreen struct {
	ssh     *cluster.SSH
	host    config.Host
	ns      string // tenant ns（fallback key 探索用）
	client  *eco.Client
	table   table.Model
	apps    []eco.App
	loading bool
	err     error
}

func NewEcoApps(ssh *cluster.SSH, host config.Host, ns string) EcoAppsScreen {
	columns := []table.Column{
		{Title: "NAME", Width: 26},
		{Title: "STATUS", Width: 8},
		{Title: "VER", Width: 4},
		{Title: "ID", Width: 38},
	}
	t := table.New(table.WithColumns(columns), table.WithFocused(true), table.WithHeight(16))
	st := table.DefaultStyles()
	st.Header = st.Header.Bold(true).BorderBottom(true).BorderForeground(lipgloss.Color("240"))
	st.Selected = st.Selected.Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("63"))
	t.SetStyles(st)

	client := eco.New(ssh, host.Helm.EcoApiKey, host.Helm.SrpName)
	client.SetFallbackNamespace(ns) // cached key 失效時自動借現役服務的 key
	return EcoAppsScreen{
		ssh:     ssh,
		host:    host,
		ns:      ns,
		client:  client,
		table:   t,
		loading: true,
	}
}

func (m EcoAppsScreen) Init() tea.Cmd { return m.fetch() }

func (m EcoAppsScreen) fetch() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		apps, err := client.ListApps(ctx)
		return ecoAppsMsg{apps: apps, err: err}
	}
}

func (m EcoAppsScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h := msg.Height - 6
		if h < 5 {
			h = 5
		}
		m.table.SetHeight(h)
		return m, nil
	case ecoAppsMsg:
		m.loading = false
		m.err = msg.err
		m.apps = msg.apps
		rows := make([]table.Row, 0, len(msg.apps))
		for _, a := range msg.apps {
			rows = append(rows, table.Row{a.Name, a.Status, fmt.Sprintf("%d", a.Version), a.ID})
		}
		m.table.SetRows(rows)
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			return m, pop()
		case "r":
			m.loading = true
			return m, m.fetch()
		case "enter":
			i := m.table.Cursor()
			if i >= 0 && i < len(m.apps) {
				return m, push(NewEcoConfig(m.ssh, m.host, m.apps[i].Name, m.ns))
			}
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m EcoAppsScreen) View() string {
	header := titleStyle.Render("eco · applications") + dimStyle.Render("  "+m.host.Name+" · srp="+m.client.SrpName)
	var status string
	switch {
	case m.err != nil:
		status = errStyle.Render("error: " + m.err.Error())
	case m.loading:
		status = statusStyle.Render("loading…")
	default:
		status = statusStyle.Render(fmt.Sprintf("apps=%d", len(m.apps)))
	}
	footer := footerStyle.Render("↑/↓ · enter view config · r refresh · esc back")
	return lipgloss.JoinVertical(lipgloss.Left, header, m.table.View(), status, footer)
}

// ---------- EcoConfigScreen：單一 app 的 config 檢視 / 編輯 / 刪除 ----------

type ecoCfgMsg struct {
	app    eco.App
	detail string
	err    error
}

// ecoEditDoneMsg：$EDITOR 關閉後回來。
type ecoEditDoneMsg struct{ err error }

type EcoConfigScreen struct {
	ssh     *cluster.SSH
	host    config.Host
	service string
	client  *eco.Client

	vp      viewport.Model
	app     eco.App
	detail  string
	loading bool
	err     error
	notice  string
	tmpFile string
}

func NewEcoConfig(ssh *cluster.SSH, host config.Host, service, ns string) EcoConfigScreen {
	client := eco.New(ssh, host.Helm.EcoApiKey, host.Helm.SrpName)
	client.SetFallbackNamespace(ns)
	return EcoConfigScreen{
		ssh:     ssh,
		host:    host,
		service: service,
		client:  client,
		vp:      viewport.New(100, 22),
		loading: true,
	}
}

func (m EcoConfigScreen) Init() tea.Cmd { return m.fetch() }

func (m EcoConfigScreen) fetch() tea.Cmd {
	client, svc := m.client, m.service
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		app, err := client.AppByName(ctx, svc)
		if err != nil {
			return ecoCfgMsg{err: err}
		}
		detail, err := client.AppDetail(ctx, app.ID)
		if err != nil {
			return ecoCfgMsg{app: app, err: err}
		}
		return ecoCfgMsg{app: app, detail: detail}
	}
}

func (m EcoConfigScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.vp.Width = msg.Width
		m.vp.Height = msg.Height - 5
		return m, nil
	case ecoCfgMsg:
		m.loading = false
		m.err = msg.err
		m.app = msg.app
		m.detail = msg.detail
		m.vp.SetContent(msg.detail)
		m.vp.GotoTop()
		return m, nil
	case ecoEditDoneMsg:
		// $EDITOR 關了，讀回檔案，有變更才推 PUT confirm
		b, err := os.ReadFile(m.tmpFile)
		if err != nil {
			m.notice = "read tmp: " + err.Error()
			return m, nil
		}
		edited := string(b)
		if edited == m.detail || edited == m.detail+"\n" {
			m.notice = "no changes — nothing sent"
			return m, nil
		}
		client, id := m.client, m.app.ID
		cmd := fmt.Sprintf("PUT /v1/applications/%s  (%d bytes)", id, len(b))
		return m, push(NewConfirm("eco update · "+m.service, cmd, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			out, err := client.UpdateApp(ctx, id, b)
			return confirmDoneMsg{out: out, err: err}
		}))
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			return m, pop()
		case "r":
			m.loading = true
			m.notice = ""
			return m, m.fetch()
		case "E":
			if m.detail == "" {
				return m, nil
			}
			f, err := os.CreateTemp("", m.service+"-eco-*.json")
			if err != nil {
				m.notice = err.Error()
				return m, nil
			}
			if _, err := f.WriteString(m.detail); err != nil {
				f.Close()
				m.notice = err.Error()
				return m, nil
			}
			f.Close()
			m.tmpFile = f.Name()
			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = "vim"
			}
			return m, tea.ExecProcess(exec.Command(editor, m.tmpFile), func(err error) tea.Msg {
				return ecoEditDoneMsg{err: err}
			})
		case "D":
			if m.app.ID == "" {
				return m, nil
			}
			client, id := m.client, m.app.ID
			cmd := fmt.Sprintf("DELETE /v1/applications/%s", id)
			return m, push(NewConfirm("eco delete · "+m.service+"  ⚠ removes ECO registration (deployment stays)", cmd, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				out, err := client.DeleteApp(ctx, id)
				return confirmDoneMsg{out: out, err: err}
			}))
		}
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

func (m EcoConfigScreen) View() string {
	header := titleStyle.Render("eco · "+m.service) + dimStyle.Render("  "+m.host.Name)

	var status string
	switch {
	case m.err != nil:
		status = errStyle.Render("✗ " + m.err.Error())
	case m.loading:
		status = statusStyle.Render("login → find app → fetch config …")
	default:
		status = statusStyle.Render(fmt.Sprintf("id=%s  status=%s  v%d", m.app.ID, m.app.Status, m.app.Version))
	}
	if m.notice != "" {
		status += "  " + dimStyle.Render("· "+m.notice)
	}

	body := m.vp.View()
	if m.loading {
		body = ""
	}

	footer := footerStyle.Render("↑/↓ scroll · r refresh · E edit in $EDITOR → PUT · D delete registration · esc back")
	return lipgloss.JoinVertical(lipgloss.Left, header, status, body, footer)
}
