package tui

import (
	"os"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/intervalrain/wedakube/internal/config"
)

// SetupScreen 讓使用者在 TUI 內填寫 FEED_PAT（遮罩），存到 secrets.json。
type SetupScreen struct {
	store    *config.Store
	input    textinput.Model
	hadValue bool
	envSet   bool
	saved    bool
}

func NewSetup(store *config.Store) SetupScreen {
	ti := textinput.New()
	ti.Placeholder = "Azure DevOps Artifacts PAT"
	ti.EchoMode = textinput.EchoPassword
	ti.EchoCharacter = '•'
	ti.CharLimit = 256
	ti.Width = 50
	ti.Focus()

	cur, _ := store.GetSecret("FEED_PAT")
	if cur != "" {
		ti.SetValue(cur)
	}
	return SetupScreen{
		store:    store,
		input:    ti,
		hadValue: cur != "",
		envSet:   os.Getenv("FEED_PAT") != "",
	}
}

func (m SetupScreen) Init() tea.Cmd { return textinput.Blink }

func (m SetupScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc":
			return m, pop()
		case "enter":
			m.store.SetSecret("FEED_PAT", m.input.Value())
			m.saved = true
			m.hadValue = m.input.Value() != ""
			return m, nil
		case "ctrl+u":
			m.input.SetValue("")
			m.saved = false
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.saved = false
	return m, cmd
}

func (m SetupScreen) View() string {
	header := titleStyle.Render("wedakube · setup")
	desc := dimStyle.Render("FEED_PAT — Azure DevOps Artifacts PAT used by docker build to restore\nprivate NuGet. Stored in ~/.k3sdeploy/secrets.json (0600).")

	field := labelStyle.Render("FEED_PAT  ") + m.input.View()

	var status string
	switch {
	case m.saved:
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("114")).Bold(true).Render("✓ saved")
	case m.envSet:
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("note: $FEED_PAT is set in your env and overrides this value")
	case m.hadValue:
		status = dimStyle.Render("a value is already stored — type to replace")
	}

	footer := footerStyle.Render("enter save · ctrl+u clear · esc back")
	return lipgloss.JoinVertical(lipgloss.Left, header, "", desc, "", field, "", status, "", footer)
}
