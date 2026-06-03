package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// confirmDoneMsg：onConfirm 跑完的結果回到 ConfirmScreen。
type confirmDoneMsg struct {
	out string
	err error
}

// ConfirmScreen 顯示「將要執行的指令」並要求 y 才執行。
// onConfirm 是 caller 提供的執行函式（已封裝完整動作 + ssh），結果以 confirmDoneMsg 回。
type ConfirmScreen struct {
	title   string
	command string
	onRun   func() tea.Msg
	out     string
	err     error
	done    bool
	running bool
}

func NewConfirm(title, command string, onRun func() tea.Msg) ConfirmScreen {
	return ConfirmScreen{title: title, command: command, onRun: onRun}
}

func (m ConfirmScreen) Init() tea.Cmd { return nil }

func (m ConfirmScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "y", "Y":
			if !m.done && !m.running && m.onRun != nil {
				m.running = true
				return m, func() tea.Msg { return m.onRun() }
			}
		case "n", "N", "esc", "q":
			return m, pop()
		}
	case confirmDoneMsg:
		m.done = true
		m.running = false
		m.out = msg.out
		m.err = msg.err
		return m, nil
	}
	return m, nil
}

func (m ConfirmScreen) View() string {
	header := titleStyle.Render(m.title)
	cmd := valStyle.Render("$ " + m.command)

	var status string
	switch {
	case m.err != nil:
		status = errStyle.Render("✗ " + m.err.Error())
	case m.done:
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("114")).Bold(true).Render("✓ done")
	case m.running:
		status = statusStyle.Render("running …")
	default:
		status = dimStyle.Render("press y to run · n / esc to cancel")
	}

	body := dimStyle.Render(m.out)
	footer := footerStyle.Render("y run · n cancel · esc back")
	return lipgloss.JoinVertical(lipgloss.Left, header, "", cmd, "", status, "", body, "", footer)
}
