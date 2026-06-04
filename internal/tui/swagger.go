package tui

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/intervalrain/wedakube/internal/cluster"
)

type swaggerReadyMsg struct {
	url       string
	localPort int
	appPort   int
}
type swaggerErrMsg struct{ err error }

// SwaggerScreen 開「ssh -L 隧道 + node 端 kubectl port-forward」二合一的進程，
// 在本機瀏覽器開到 http://localhost:<port>/swagger。esc 同時收掉兩端。
type SwaggerScreen struct {
	ssh     *cluster.SSH
	ns      string
	service string
	appPort int // 0 = 自動偵測 deploy 的 containerPort

	url       string
	localPort int
	realPort  int
	ctx       context.Context
	cancel    context.CancelFunc
	ready     bool
	err       error
}

func NewSwaggerScreen(ssh *cluster.SSH, ns, service string, appPort int) SwaggerScreen {
	ctx, cancel := context.WithCancel(context.Background())
	return SwaggerScreen{
		ssh: ssh, ns: ns, service: service,
		appPort: appPort,
		ctx:     ctx, cancel: cancel,
	}
}

func (m SwaggerScreen) Init() tea.Cmd {
	return m.start()
}

func (m SwaggerScreen) start() tea.Cmd {
	return func() tea.Msg {
		port := m.appPort
		if port == 0 {
			kc := cluster.NewKubectl(m.ssh, m.ns)
			ctx, cancel := context.WithTimeout(m.ctx, 8*time.Second)
			if p, err := kc.ContainerPort(ctx, m.service); err == nil && p > 0 {
				port = p
			}
			cancel()
			if port == 0 {
				port = 5001
			}
		}

		localPort := pickLocalPort(18080, 18180)
		remoteCmd := fmt.Sprintf("kubectl -n %s port-forward deploy/%s %d:%d", m.ns, m.service, localPort, port)
		if err := m.ssh.PortForward(m.ctx, localPort, localPort, remoteCmd); err != nil {
			return swaggerErrMsg{err}
		}

		if err := waitForPort(m.ctx, localPort, 6*time.Second); err != nil {
			return swaggerErrMsg{fmt.Errorf("tunnel not ready: %w", err)}
		}

		url := fmt.Sprintf("http://localhost:%d/swagger", localPort)
		_ = openBrowser(url)
		return swaggerReadyMsg{url: url, localPort: localPort, appPort: port}
	}
}

func (m SwaggerScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case swaggerReadyMsg:
		m.url = msg.url
		m.localPort = msg.localPort
		m.realPort = msg.appPort
		m.ready = true
		return m, nil
	case swaggerErrMsg:
		m.err = msg.err
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			m.cancel()
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
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("114")).Bold(true).Render("● TUNNEL OPEN")
	default:
		status = statusStyle.Render("opening tunnel …")
	}

	var body string
	if m.ready {
		body = valStyle.Render(m.url) + "\n\n" +
			labelStyle.Render(fmt.Sprintf("ns=%s    pod-port=%d    localhost=%d", m.ns, m.realPort, m.localPort))
	}

	footer := footerStyle.Render("o reopen browser · esc close tunnel & back")
	return lipgloss.JoinVertical(lipgloss.Left, header, "", status, "", body, "", footer)
}

// pickLocalPort 找一個本機可 bind 的 port（給 ssh -L 用）。
func pickLocalPort(start, end int) int {
	for p := start; p <= end; p++ {
		l, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", p))
		if err == nil {
			_ = l.Close()
			return p
		}
	}
	return start
}

// waitForPort 探本機 port 直到能連上（代表 ssh -L 接通了）。
func waitForPort(ctx context.Context, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), 500*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
	return fmt.Errorf("port %d not ready after %v", port, timeout)
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
