package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/intervalrain/wedakube/internal/config"
)

// NamespaceEditor 讓 user 在 L2 直接改 host.Namespace（持久化到 state.json）。
// 空字串 = 回到 auto-derive。
type NamespaceEditor struct {
	store    *config.Store
	hostName string
	input    textinput.Model
	err      error
}

func NewNamespaceEditor(store *config.Store, hostName, current string) NamespaceEditor {
	ti := textinput.New()
	ti.Placeholder = "namespace (empty = auto-derive)"
	ti.SetValue(current)
	ti.CharLimit = 200
	ti.Width = 60
	ti.CursorEnd()
	ti.Focus()
	return NamespaceEditor{store: store, hostName: hostName, input: ti}
}

func (m NamespaceEditor) Init() tea.Cmd { return textinput.Blink }

func (m NamespaceEditor) Update(msg tea.Msg) (screen, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc":
			return m, pop()
		case "enter":
			h, ok, _ := m.store.GetHost(m.hostName)
			if !ok {
				m.err = fmt.Errorf("host %s not found", m.hostName)
				return m, nil
			}
			h.Namespace = strings.TrimSpace(m.input.Value())
			if err := m.store.PutHost(h); err != nil {
				m.err = err
				return m, nil
			}
			return m, pop()
		case "ctrl+u":
			m.input.SetValue("")
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m NamespaceEditor) View() string {
	header := titleStyle.Render("edit namespace · " + m.hostName)
	desc := dimStyle.Render("L2 will refetch services from this namespace.\nEmpty = auto-derive (<tenantId>-<srpName>, or fallback to *-weda).")
	field := labelStyle.Render("Namespace ") + m.input.View()

	var status string
	if m.err != nil {
		status = errStyle.Render("✗ " + m.err.Error())
	}
	footer := footerStyle.Render("enter save · ctrl+u clear · esc cancel")
	return lipgloss.JoinVertical(lipgloss.Left, header, "", desc, "", field, "", status, "", footer)
}
