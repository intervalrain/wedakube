package tui

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/intervalrain/wedakube/internal/cluster"
)

type swaggerReadyMsg struct {
	url      string
	nodePort int
	nodeIP   string
}
type swaggerErrMsg struct{ err error }
type revertedMsg struct{ err error }

// SwaggerScreen 暫時把 svc patch 成 NodePort，組出 http://<node-ip>:<nodePort>/swagger
// 在本機瀏覽器開。esc 自動 patch 回 ClusterIP（避免在共用 dev 留下暴露的 port）。
type SwaggerScreen struct {
	kc      *cluster.Kubectl
	service string

	url      string
	nodePort int
	nodeIP   string
	ctx      context.Context
	cancel   context.CancelFunc
	ready    bool
	err      error
	reverted bool
	revertOK bool
}

func NewSwaggerScreen(kc *cluster.Kubectl, service string) SwaggerScreen {
	ctx, cancel := context.WithCancel(context.Background())
	return SwaggerScreen{kc: kc, service: service, ctx: ctx, cancel: cancel}
}

func (m SwaggerScreen) Init() tea.Cmd { return m.expose() }

func (m SwaggerScreen) expose() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
		defer cancel()

		if _, err := m.kc.PatchServiceType(ctx, m.service, "NodePort"); err != nil {
			return swaggerErrMsg{err}
		}
		// 給 k3s 一拍時間分配 nodePort
		time.Sleep(400 * time.Millisecond)

		port, err := m.kc.NodePort(ctx, m.service)
		if err != nil {
			return swaggerErrMsg{fmt.Errorf("read nodePort: %w", err)}
		}
		ip, err := m.kc.NodeIP(ctx)
		if err != nil {
			return swaggerErrMsg{fmt.Errorf("read node IP: %w", err)}
		}

		url := fmt.Sprintf("http://%s:%d/swagger", ip, port)
		_ = openBrowser(url)
		return swaggerReadyMsg{url: url, nodePort: port, nodeIP: ip}
	}
}

func (m SwaggerScreen) revert() tea.Cmd {
	kc := m.kc
	svc := m.service
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err := kc.PatchServiceType(ctx, svc, "ClusterIP")
		return revertedMsg{err: err}
	}
}

func (m SwaggerScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case swaggerReadyMsg:
		m.url = msg.url
		m.nodePort = msg.nodePort
		m.nodeIP = msg.nodeIP
		m.ready = true
		return m, nil
	case swaggerErrMsg:
		m.err = msg.err
		return m, nil
	case revertedMsg:
		m.reverted = true
		m.revertOK = msg.err == nil
		if !m.revertOK {
			m.err = msg.err
		}
		return m, pop()
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			if m.ready && !m.reverted {
				// 還在 NodePort 狀態 → 先 revert 再 pop
				return m, m.revert()
			}
			return m, pop()
		case "o":
			if m.ready {
				_ = openBrowser(m.url)
			}
			return m, nil
		}
	}
	return m, nil
}

func (m SwaggerScreen) View() string {
	header := titleStyle.Render("swagger · " + m.service)

	var status string
	switch {
	case m.err != nil:
		status = errStyle.Render("✗ " + m.err.Error())
	case m.ready:
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true).Render("● EXPOSED via NodePort  ") +
			dimStyle.Render("(esc reverts to ClusterIP)")
	default:
		status = statusStyle.Render("patching svc to NodePort …")
	}

	var body string
	if m.ready {
		body = valStyle.Render(m.url) + "\n\n" +
			labelStyle.Render(fmt.Sprintf("node=%s    nodePort=%d", m.nodeIP, m.nodePort))
	}

	footer := footerStyle.Render("o reopen browser · esc revert & back")
	return lipgloss.JoinVertical(lipgloss.Left, header, "", status, "", body, "", footer)
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	}
	return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
}
