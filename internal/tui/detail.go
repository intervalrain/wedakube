package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/intervalrain/wedakube/internal/cluster"
	"github.com/intervalrain/wedakube/internal/config"
	"github.com/intervalrain/wedakube/internal/model"
)

// helmReleaseMsg：detail 背景偵測完該 deployment 是哪個 helm release 的回報。
type helmReleaseMsg struct {
	release, namespace string
}

// execDoneMsg：tea.ExecProcess 回來時觸發；err 多半是 0 / nil（user 正常 exit）。
type execDoneMsg struct{ err error }

var (
	keyStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Bold(true)
	groupStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("114")).Bold(true)
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	valStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("231"))
)

// 寫操作鎖死的 namespace（README §9 安全規則）。
var protectedNamespaces = map[string]bool{
	"kube-system":          true,
	"wise-system":          true,
	"wise-backing-service": true,
	"wise-observability":   true,
}

func isProtectedNS(ns string) bool { return protectedNamespaces[ns] }

// deployPrereqsMissing 回傳 target 部署前缺哪些檔（K3S 部署 procedure 的硬性前置）。
// 兩個都到 = D 才會亮。
func deployPrereqsMissing(t *config.Target) []string {
	if t == nil {
		return nil
	}
	var missing []string
	df := t.Dockerfile
	if df == "" {
		df = filepath.Join(t.Context, "Dockerfile")
	}
	if _, err := os.Stat(df); err != nil {
		missing = append(missing, "Dockerfile")
	}
	if _, err := os.Stat(filepath.Join(t.Context, "appcfg.yaml")); err != nil {
		missing = append(missing, "appcfg.yaml")
	}
	return missing
}

// ServiceDetail 是 L3：選定一個服務後的摘要 + 動作選單。
type ServiceDetail struct {
	kubectl    *cluster.Kubectl
	host       string
	store      *config.Store
	svc        model.Service
	target     *config.Target // 非 nil = 此服務有部署目標，可按 D
	release    string         // helm release 名（空 = 不是 helm 管的，X 不亮）
	releaseNS  string
}

func NewServiceDetail(kc *cluster.Kubectl, host string, store *config.Store, svc model.Service, target *config.Target) ServiceDetail {
	return ServiceDetail{kubectl: kc, host: host, store: store, svc: svc, target: target}
}

func (m ServiceDetail) Init() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		rel, ns, _ := m.kubectl.HelmReleaseFor(ctx, m.svc.Name)
		return helmReleaseMsg{release: rel, namespace: ns}
	}
}

func (m ServiceDetail) Update(msg tea.Msg) (screen, tea.Cmd) {
	if rm, ok := msg.(helmReleaseMsg); ok {
		m.release = rm.release
		m.releaseNS = rm.namespace
		return m, nil
	}
	if _, ok := msg.(execDoneMsg); ok {
		// shell 結束、TUI 回來，不用做任何事
		return m, nil
	}
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	name := m.svc.Name
	sel := "app.kubernetes.io/name=" + name
	switch k.String() {
	case "esc", "q":
		return m, pop()
	case "w":
		return m, push(NewSwaggerScreen(m.kubectl, name))
	case "x":
		if isProtectedNS(m.kubectl.Namespace()) {
			return m, nil
		}
		remote := fmt.Sprintf("kubectl -n %s exec -it deploy/%s -- sh", m.kubectl.Namespace(), name)
		sshCmd := m.kubectl.SSH().ExecCmd(context.Background(), remote)
		return m, tea.ExecProcess(sshCmd, func(err error) tea.Msg {
			return execDoneMsg{err: err}
		})
	case "R":
		if isProtectedNS(m.kubectl.Namespace()) {
			return m, nil
		}
		kc := m.kubectl
		svc := name
		cmd := fmt.Sprintf("kubectl -n %s rollout restart deploy/%s", kc.Namespace(), svc)
		return m, push(NewConfirm("restart · "+svc, cmd, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			out, err := kc.RolloutRestart(ctx, svc)
			return confirmDoneMsg{out: out, err: err}
		}))
	case "+":
		if isProtectedNS(m.kubectl.Namespace()) {
			return m, nil
		}
		kc := m.kubectl
		svc := name
		cmd := fmt.Sprintf("kubectl -n %s scale deploy/%s --replicas=1", kc.Namespace(), svc)
		return m, push(NewConfirm("start · "+svc, cmd, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			out, err := kc.Scale(ctx, svc, 1)
			return confirmDoneMsg{out: out, err: err}
		}))
	case "-":
		if isProtectedNS(m.kubectl.Namespace()) {
			return m, nil
		}
		kc := m.kubectl
		svc := name
		cmd := fmt.Sprintf("kubectl -n %s scale deploy/%s --replicas=0", kc.Namespace(), svc)
		return m, push(NewConfirm("stop · "+svc, cmd, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			out, err := kc.Scale(ctx, svc, 0)
			return confirmDoneMsg{out: out, err: err}
		}))
	case "z":
		if isProtectedNS(m.kubectl.Namespace()) {
			return m, nil
		}
		kc := m.kubectl
		svc := name
		// helm-managed 走 helm rollback；否則走 kubectl rollout undo
		if m.release != "" {
			rel, ns := m.release, m.releaseNS
			cmd := fmt.Sprintf("helm rollback %s -n %s", rel, ns)
			return m, push(NewConfirm("rollback · "+svc, cmd, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				defer cancel()
				out, err := kc.HelmRollback(ctx, rel, ns)
				return confirmDoneMsg{out: out, err: err}
			}))
		}
		cmd := fmt.Sprintf("kubectl -n %s rollout undo deploy/%s", kc.Namespace(), svc)
		return m, push(NewConfirm("rollback · "+svc, cmd, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			out, err := kc.RolloutUndo(ctx, svc)
			return confirmDoneMsg{out: out, err: err}
		}))
	case "X":
		if m.release == "" || isProtectedNS(m.kubectl.Namespace()) {
			return m, nil
		}
		kc := m.kubectl
		rel, ns := m.release, m.releaseNS
		cmd := fmt.Sprintf("helm uninstall %s -n %s", rel, ns)
		return m, push(NewConfirm("uninstall · "+name, cmd, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			out, err := kc.HelmUninstall(ctx, rel, ns)
			return confirmDoneMsg{out: out, err: err}
		}))
	case "s":
		return m, push(NewTextScreen(m.kubectl, "status · "+name,
			fmt.Sprintf("get deploy,rs,pod -l %s -o wide", sel)))
	case "i":
		return m, push(NewTextScreen(m.kubectl, "info · "+name, "describe deploy "+name))
	case "l":
		return m, push(NewLogsScreen(m.kubectl.SSH(), m.kubectl.Namespace(), name))
	case "u":
		return m, push(NewTextScreen(m.kubectl, "resource · "+name, "top pod -l "+sel))
	case "k":
		return m, push(NewTextScreen(m.kubectl, "networking", "get svc"))
	case "D":
		if m.target == nil || isProtectedNS(m.kubectl.Namespace()) {
			return m, nil
		}
		if len(deployPrereqsMissing(m.target)) > 0 {
			return m, nil // 缺檔，灰著按不動
		}
		return m, push(NewDeployScreen(m.kubectl.SSH(), m.store, *m.target))
	}
	return m, nil
}

func (m ServiceDetail) View() string {
	s := m.svc
	header := titleStyle.Render(s.Name) + dimStyle.Render("  "+m.host+" · "+m.kubectl.Namespace())

	kv := func(label, val string) string {
		return labelStyle.Render(fmt.Sprintf("%-12s", label)) + valStyle.Render(val)
	}
	summary := lipgloss.JoinVertical(lipgloss.Left,
		kv("image", s.Image),
		kv("ready", s.Ready)+"   "+kv("up-to-date", fmt.Sprintf("%d", s.UpToDate))+"   "+kv("age", s.Age),
	)

	item := func(key, desc string) string {
		return "  " + keyStyle.Render(key) + " " + labelStyle.Render(desc)
	}
	inspect := groupStyle.Render("INSPECT") + dimStyle.Render("  read-only") + "\n" +
		item("s", "status") + item("i", "info") + item("l", "logs") + item("u", "resource") + item("k", "networking")
	protected := isProtectedNS(m.kubectl.Namespace())

	keyOrDim := func(active bool, key, label string) string {
		if active {
			return item(key, label)
		}
		return dimStyle.Render("  " + key + " " + label)
	}

	missingPrereqs := deployPrereqsMissing(m.target)
	canDeploy := !protected && m.target != nil && len(missingPrereqs) == 0
	canWrite := !protected
	deployStr := keyOrDim(canDeploy, "D", "deploy")
	restartStr := keyOrDim(canWrite, "R", "restart")
	startStr := keyOrDim(canWrite, "+", "start")
	stopStr := keyOrDim(canWrite, "-", "stop")
	rollbackStr := keyOrDim(canWrite, "z", "rollback")
	deployLine := deployStr + "  " + restartStr + "  " + startStr + "  " + stopStr + "  " + rollbackStr

	var uninstallLine string
	switch {
	case protected:
		uninstallLine = "\n" + dimStyle.Render("  X helm uninstall (locked: protected ns)")
	case m.release != "":
		uninstallLine = "\n" + item("X", "helm uninstall  ") + dimStyle.Render("("+m.release+" in "+m.releaseNS+")")
	default:
		uninstallLine = "\n" + dimStyle.Render("  X helm uninstall — only for Helm-managed services")
	}

	hint := "  build + push + rollout · scale · restart · rollback"
	switch {
	case protected:
		hint = "  ⚠ protected namespace — writes disabled"
	case m.target == nil:
		hint = "  no pin/repo — add one in L2 to enable D deploy"
	case len(missingPrereqs) > 0:
		hint = "  ⚠ repo missing: " + strings.Join(missingPrereqs, ", ") + " — add them to enable D"
	}
	lifecycle := groupStyle.Render("LIFECYCLE") + dimStyle.Render(hint) + "\n" + deployLine + uninstallLine
	xStr := keyOrDim(canWrite, "x", "exec")
	debug := groupStyle.Render("DEBUG") + dimStyle.Render("  shell into pod · swagger via NodePort") + "\n" +
		xStr + "  " + dimStyle.Render("f port-forward (coming)") + "  " + item("w", "swagger")

	footer := footerStyle.Render("press a key · esc back · ctrl+c quit")

	return lipgloss.JoinVertical(lipgloss.Left,
		header, "", summary, "", inspect, "", lifecycle, "", debug, footer)
}
