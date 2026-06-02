package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/intervalrain/wedakube/internal/cluster"
	"github.com/intervalrain/wedakube/internal/model"
)

var (
	keyStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Bold(true)
	groupStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("114")).Bold(true)
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	valStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("231"))
)

// ServiceDetail 是 L3：選定一個服務後的摘要 + 動作選單。
type ServiceDetail struct {
	kubectl *cluster.Kubectl
	host    string
	svc     model.Service
}

func NewServiceDetail(kc *cluster.Kubectl, host string, svc model.Service) ServiceDetail {
	return ServiceDetail{kubectl: kc, host: host, svc: svc}
}

func (m ServiceDetail) Init() tea.Cmd { return nil }

func (m ServiceDetail) Update(msg tea.Msg) (screen, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	name := m.svc.Name
	sel := "app.kubernetes.io/name=" + name
	switch k.String() {
	case "esc", "q":
		return m, pop()
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
	lifecycle := groupStyle.Render("LIFECYCLE") + dimStyle.Render("  needs WRITE mode — coming next") + "\n" +
		dimStyle.Render("  D deploy   R restart   ↑ start   ↓ stop   z rollback")
	debug := groupStyle.Render("DEBUG") + dimStyle.Render("  coming next") + "\n" +
		dimStyle.Render("  x exec   f port-forward   w swagger")

	footer := footerStyle.Render("press a key · esc back · ctrl+c quit")

	return lipgloss.JoinVertical(lipgloss.Left,
		header, "", summary, "", inspect, "", lifecycle, "", debug, footer)
}
