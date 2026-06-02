package tui

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/intervalrain/wedakube/internal/cluster"
)

type textMsg struct{ s string }

// TextScreen 是一個可捲動的唯讀文字檢視（logs / status / describe / top / get svc 共用）。
type TextScreen struct {
	kubectl *cluster.Kubectl
	title   string
	args    string
	vp      viewport.Model
	loading bool
}

func NewTextScreen(kc *cluster.Kubectl, title, args string) TextScreen {
	return TextScreen{
		kubectl: kc,
		title:   title,
		args:    args,
		vp:      viewport.New(100, 22),
		loading: true,
	}
}

func (m TextScreen) Init() tea.Cmd { return m.fetch() }

func (m TextScreen) fetch() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		out, err := m.kubectl.Raw(ctx, m.args)
		if err != nil && out == "" {
			out = err.Error()
		}
		return textMsg{out}
	}
}

func (m TextScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.vp.Width = msg.Width
		m.vp.Height = msg.Height - 3
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			return m, pop()
		case "r":
			m.loading = true
			return m, m.fetch()
		}
	case textMsg:
		m.loading = false
		m.vp.SetContent(msg.s)
		m.vp.GotoTop()
		return m, nil
	}

	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

func (m TextScreen) View() string {
	header := titleStyle.Render(m.title)
	body := m.vp.View()
	if m.loading {
		body = statusStyle.Render("loading…")
	}
	footer := footerStyle.Render("↑/↓ scroll · r refresh · esc back")
	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}
