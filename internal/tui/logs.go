package tui

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/intervalrain/wedakube/internal/cluster"
)

type logLine string
type streamEndMsg struct{}

const maxLogLines = 5000

// LogsScreen 用 ssh + kubectl logs -f 即時串流，預設 FOLLOW；f 可暫停讓你往上看。
type LogsScreen struct {
	ssh     *cluster.SSH
	ns      string
	service string

	vp     viewport.Model
	lines  []string
	follow bool
	closed bool

	ctx    context.Context
	cancel context.CancelFunc
	ch     chan logLine
}

func NewLogsScreen(ssh *cluster.SSH, ns, service string) LogsScreen {
	ctx, cancel := context.WithCancel(context.Background())
	return LogsScreen{
		ssh: ssh, ns: ns, service: service,
		vp:     viewport.New(100, 22),
		follow: true,
		ctx:    ctx, cancel: cancel,
		ch: make(chan logLine, 1024),
	}
}

func (m LogsScreen) Init() tea.Cmd {
	go m.streamLines()
	return m.waitLine()
}

func (m LogsScreen) streamLines() {
	defer close(m.ch)
	cmd := fmt.Sprintf("kubectl -n %s logs deploy/%s -f --tail=200 2>&1", m.ns, m.service)
	r, err := m.ssh.Stream(m.ctx, cmd)
	if err != nil {
		select {
		case m.ch <- logLine("[error] " + err.Error()):
		case <-m.ctx.Done():
		}
		return
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		select {
		case m.ch <- logLine(sc.Text()):
		case <-m.ctx.Done():
			return
		}
	}
}

func (m LogsScreen) waitLine() tea.Cmd {
	return func() tea.Msg {
		v, ok := <-m.ch
		if !ok {
			return streamEndMsg{}
		}
		return v
	}
}

func (m LogsScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.vp.Width = msg.Width
		m.vp.Height = msg.Height - 3
		if m.follow {
			m.vp.GotoBottom()
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			m.cancel()
			return m, pop()
		case "f":
			m.follow = !m.follow
			if m.follow {
				m.vp.GotoBottom()
			}
			return m, nil
		case "r":
			m.lines = nil
			m.vp.SetContent("")
			return m, nil
		}
	case logLine:
		m.lines = append(m.lines, string(msg))
		if len(m.lines) > maxLogLines {
			m.lines = m.lines[len(m.lines)-maxLogLines:]
		}
		m.vp.SetContent(strings.Join(m.lines, "\n"))
		if m.follow {
			m.vp.GotoBottom()
		}
		return m, m.waitLine()
	case streamEndMsg:
		m.closed = true
		return m, nil
	}

	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

func (m LogsScreen) View() string {
	state := lipgloss.NewStyle().Foreground(lipgloss.Color("114")).Bold(true).Render("FOLLOW")
	switch {
	case m.closed:
		state = errStyle.Render("ENDED")
	case !m.follow:
		state = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("PAUSED")
	}
	header := titleStyle.Render("logs · "+m.service) + "  " + state +
		dimStyle.Render("   "+fmt.Sprintf("%d lines", len(m.lines)))
	footer := footerStyle.Render("f toggle follow · r clear · ↑/↓ scroll · esc back")
	return lipgloss.JoinVertical(lipgloss.Left, header, m.vp.View(), footer)
}
