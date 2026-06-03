package tui

import (
	"context"
	"fmt"
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

var (
	keyStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Bold(true)
	groupStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("114")).Bold(true)
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	valStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("231"))
)

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
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	name := m.svc.Name
	sel := "app.kubernetes.io/name=" + name
	switch k.String() {
	case "esc", "q":
		return m, pop()
	case "X":
		if m.release == "" {
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
		return m, push(NewTextScreen(m.kubectl, "logs · "+name, "logs deploy/"+name+" --tail=300"))
	case "u":
		return m, push(NewTextScreen(m.kubectl, "resource · "+name, "top pod -l "+sel))
	case "k":
		return m, push(NewTextScreen(m.kubectl, "networking", "get svc"))
	case "D":
		if m.target != nil {
			return m, push(NewDeployScreen(m.kubectl.SSH(), m.store, *m.target))
		}
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
	var lifecycle string
	deployLine := dimStyle.Render("  D deploy   R restart   ↑ start   ↓ stop   z rollback")
	if m.target != nil {
		deployLine = item("D", "deploy") + dimStyle.Render("  R restart   ↑ start   ↓ stop   z rollback (coming next)")
	}
	var uninstallLine string
	if m.release != "" {
		uninstallLine = "\n" + item("X", "helm uninstall  ") + dimStyle.Render("("+m.release+" in "+m.releaseNS+")")
	} else {
		uninstallLine = "\n" + dimStyle.Render("  X helm uninstall — only for Helm-managed services")
	}
	hint := "  build + push + rollout this repo"
	if m.target == nil {
		hint = "  no pin/repo — add one in L2 to deploy"
	}
	lifecycle = groupStyle.Render("LIFECYCLE") + dimStyle.Render(hint) + "\n" + deployLine + uninstallLine
	debug := groupStyle.Render("DEBUG") + dimStyle.Render("  coming next") + "\n" +
		dimStyle.Render("  x exec   f port-forward   w swagger")

	footer := footerStyle.Render("press a key · esc back · ctrl+c quit")

	return lipgloss.JoinVertical(lipgloss.Left,
		header, "", summary, "", inspect, "", lifecycle, "", debug, footer)
}
