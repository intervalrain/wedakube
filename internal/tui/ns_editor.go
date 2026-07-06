package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/intervalrain/wedakube/internal/cluster"
	"github.com/intervalrain/wedakube/internal/config"
)

// nsListMsg 是從 cluster 撈回 namespace 清單的回報（picker 共用）。
type nsListMsg struct {
	names []string
	err   error
}

// fetchNamespaces 是 ns picker 共用的抓取 cmd。
func fetchNamespaces(ssh *cluster.SSH) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		names, err := cluster.ListNamespaces(ctx, ssh)
		return nsListMsg{names: names, err: err}
	}
}

// newNSTable 建 picker 表格（editor / wizard 共用樣式）。
func newNSTable() table.Model {
	t := table.New(
		table.WithColumns([]table.Column{
			{Title: "", Width: 3},
			{Title: "NAMESPACE", Width: 52},
			{Title: "", Width: 14},
		}),
		table.WithFocused(true),
		table.WithHeight(14),
	)
	st := table.DefaultStyles()
	st.Header = st.Header.Bold(true).BorderBottom(true).BorderForeground(lipgloss.Color("240"))
	st.Selected = st.Selected.Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("63"))
	t.SetStyles(st)
	return t
}

// nsRows 把 namespace 名單轉成表格列。首列是 auto-derive；protected 標鎖。
// withAuto = editor 才有（wizard 一定要具體 ns）。
func nsRows(names []string, current string, withAuto bool) []table.Row {
	rows := []table.Row{}
	if withAuto {
		mark := ""
		if current == "" {
			mark = "●"
		}
		rows = append(rows, table.Row{mark, "(auto-derive)", ""})
	}
	for _, n := range names {
		mark := ""
		if n == current {
			mark = "●"
		}
		note := ""
		if isProtectedNS(n) {
			note = "🔒 read-only"
		}
		rows = append(rows, table.Row{mark, n, note})
	}
	return rows
}

// NamespaceEditor：L2 按 N。從 cluster 挑一個 namespace（或 auto-derive），
// 持久化到 host.Namespace。按 i 可切手動輸入（要指定還不存在的 ns 時用）。
type NamespaceEditor struct {
	store    *config.Store
	ssh      *cluster.SSH
	hostName string
	current  string

	table   table.Model
	names   []string
	loading bool
	manual  bool
	input   textinput.Model
	err     error
}

func NewNamespaceEditor(store *config.Store, ssh *cluster.SSH, hostName, current string) NamespaceEditor {
	ti := textinput.New()
	ti.Placeholder = "type a namespace (may not exist yet)"
	ti.SetValue(current)
	ti.CharLimit = 200
	ti.Width = 60
	ti.CursorEnd()
	return NamespaceEditor{
		store: store, ssh: ssh, hostName: hostName, current: current,
		table: newNSTable(), loading: true, input: ti,
	}
}

func (m NamespaceEditor) Init() tea.Cmd { return fetchNamespaces(m.ssh) }

func (m NamespaceEditor) save(ns string) (screen, tea.Cmd) {
	h, ok, _ := m.store.GetHost(m.hostName)
	if !ok {
		m.err = fmt.Errorf("host %s not found", m.hostName)
		return m, nil
	}
	h.Namespace = strings.TrimSpace(ns)
	if err := m.store.PutHost(h); err != nil {
		m.err = err
		return m, nil
	}
	return m, pop()
}

func (m NamespaceEditor) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h := msg.Height - 8
		if h < 5 {
			h = 5
		}
		m.table.SetHeight(h)
		return m, nil
	case nsListMsg:
		m.loading = false
		m.err = msg.err
		m.names = msg.names
		m.table.SetRows(nsRows(msg.names, m.current, true))
		// 游標停在目前選的 ns 上
		for i, n := range msg.names {
			if n == m.current {
				m.table.SetCursor(i + 1) // +1 跳過 auto-derive 列
			}
		}
		return m, nil
	case tea.KeyMsg:
		if m.manual {
			switch msg.String() {
			case "esc":
				m.manual = false
				return m, nil
			case "enter":
				return m.save(m.input.Value())
			case "ctrl+u":
				m.input.SetValue("")
				return m, nil
			}
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
		switch msg.String() {
		case "esc", "q":
			return m, pop()
		case "r":
			m.loading = true
			return m, fetchNamespaces(m.ssh)
		case "i":
			m.manual = true
			m.input.Focus()
			return m, textinput.Blink
		case "enter":
			i := m.table.Cursor()
			if i == 0 {
				return m.save("") // auto-derive
			}
			if i >= 1 && i-1 < len(m.names) {
				return m.save(m.names[i-1])
			}
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m NamespaceEditor) View() string {
	header := titleStyle.Render("namespace · " + m.hostName)
	desc := dimStyle.Render("Pick the namespace L2 lists services from. ● = current · auto-derive = <tenantId>-<srpName>")

	if m.manual {
		field := labelStyle.Render("Namespace ") + m.input.View()
		footer := footerStyle.Render("enter save · ctrl+u clear · esc back to list")
		return lipgloss.JoinVertical(lipgloss.Left, header, "", desc, "", field, "", footer)
	}

	var status string
	switch {
	case m.err != nil:
		status = errStyle.Render("✗ " + m.err.Error())
	case m.loading:
		status = statusStyle.Render("loading namespaces…")
	default:
		status = statusStyle.Render(fmt.Sprintf("namespaces=%d", len(m.names)))
	}
	footer := footerStyle.Render("↑/↓ · enter select · i type manually · r refresh · esc cancel")
	return lipgloss.JoinVertical(lipgloss.Left, header, "", desc, m.table.View(), status, footer)
}
