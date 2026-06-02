package tui

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/intervalrain/wedakube/internal/cluster"
	"github.com/intervalrain/wedakube/internal/config"
	"github.com/intervalrain/wedakube/internal/deploy"
)

// endMsg 由部署 goroutine 在結束時送出（成功 err 為 nil）。
type endMsg struct{ err error }

// DeployScreen 跑部署引擎並即時呈現進度條 + log。
type DeployScreen struct {
	target config.Target
	ssh    *cluster.SSH
	store  *config.Store
	events chan deploy.Event
	endCh  chan error
	bar       progress.Model
	lines     []string
	failDetail string
	phase     string
	done      bool
	err       error
}

func NewDeployScreen(ssh *cluster.SSH, store *config.Store, t config.Target) DeployScreen {
	bar := progress.New(progress.WithScaledGradient("#9ed12e", "#ccff45"), progress.WithoutPercentage())
	bar.Width = 48
	return DeployScreen{
		target: t,
		ssh:    ssh,
		store:  store,
		events: make(chan deploy.Event, 256),
		endCh:  make(chan error, 1),
		bar:    bar,
	}
}

func (m DeployScreen) Init() tea.Cmd {
	go func() {
		emit := func(e deploy.Event) { m.events <- e }
		date := time.Now().Format("20060102")
		feedPAT := deploy.ResolveFeedPAT(m.store)
		_, err := deploy.Deploy(context.Background(), m.ssh, m.store, m.target, date, feedPAT, true, emit)
		m.endCh <- err
	}()
	return tea.Batch(waitEvent(m.events), waitEnd(m.endCh))
}

func waitEvent(ch chan deploy.Event) tea.Cmd {
	return func() tea.Msg { return <-ch }
}
func waitEnd(ch chan error) tea.Cmd {
	return func() tea.Msg { return endMsg{<-ch} }
}

func (m DeployScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "esc" || msg.String() == "q" {
			return m, pop()
		}
	case deploy.Event:
		m.phase = msg.Phase
		if msg.Phase == "fail" {
			m.failDetail = msg.Msg // 完整 describe + logs，View 會顯示
			m.lines = append(m.lines, "[fail] captured pod diagnostics ↓")
		} else {
			m.lines = append(m.lines, "["+msg.Phase+"] "+firstLine(msg.Msg))
		}
		if len(m.lines) > 14 {
			m.lines = m.lines[len(m.lines)-14:]
		}
		var cmd tea.Cmd
		if msg.Pct >= 0 {
			cmd = m.bar.SetPercent(msg.Pct)
		}
		return m, tea.Batch(cmd, waitEvent(m.events))
	case endMsg:
		m.done = true
		m.err = msg.err
		if m.err == nil {
			return m, m.bar.SetPercent(1)
		}
		return m, nil
	case progress.FrameMsg:
		pm, cmd := m.bar.Update(msg)
		m.bar = pm.(progress.Model)
		return m, cmd
	}
	return m, nil
}

func (m DeployScreen) View() string {
	t := m.target
	header := titleStyle.Render("deploy · " + t.Service)
	sub := dimStyle.Render(t.ImageRepo + "  →  " + t.Namespace + " @ " + hostOf(t))

	var statusLine string
	switch {
	case m.err != nil:
		statusLine = errStyle.Render("✗ failed: " + firstLine(m.err.Error()))
	case m.done:
		statusLine = lipgloss.NewStyle().Foreground(lipgloss.Color("114")).Bold(true).Render("✓ deployed")
	default:
		statusLine = labelStyle.Render("phase: ") + valStyle.Render(m.phase)
	}

	bar := m.bar.View()

	log := dimStyle.Render(strings.Join(m.lines, "\n"))

	if m.err != nil && m.failDetail != "" {
		log = log + "\n\n" + dimStyle.Render(tailLines(m.failDetail, 18))
	}

	hint := ""
	if m.done {
		hint = dimStyle.Render("esc back · ctrl+c quit")
	} else {
		hint = dimStyle.Render("deploying… esc to background")
	}
	footer := footerStyle.Render(hint)

	return lipgloss.JoinVertical(lipgloss.Left,
		header, sub, "", bar, statusLine, "", log, "", footer)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// tailLines 回傳字串最後 n 行（失敗診斷太長時只秀尾巴）。
func tailLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func hostOf(t config.Target) string {
	if t.Host != "" {
		return t.Host
	}
	return t.SSHAlias
}
