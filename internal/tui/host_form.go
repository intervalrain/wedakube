package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/intervalrain/wedakube/internal/config"
)

type hostField struct{ key, label, hint, value string }

// HostFormScreen 新增 / 編輯 host。L1 按 n 進新建 mode、按 e 進編輯 mode。
type HostFormScreen struct {
	store    *config.Store
	isEdit   bool
	original string // 編輯前的 Name，支援改名
	fields   []hostField
	idx      int
	input    textinput.Model
	err      error
}

func NewHostForm(store *config.Store, existing *config.Host) HostFormScreen {
	fields := []hostField{
		{key: "name", label: "Name", hint: "display name (also ControlMaster key)"},
		{key: "host", label: "HostName/IP", hint: "node IP or resolvable hostname"},
		{key: "user", label: "User", hint: "ssh user (often ubuntu / root)"},
		{key: "key", label: "IdentityFile", hint: "~/.ssh/id_ed25519 (leave empty to use ~/.ssh/config)"},
		{key: "ns", label: "Namespace", hint: "leave empty to auto-derive from cluster helm params"},
	}
	original := ""
	if existing != nil {
		setHostField(fields, "name", existing.Name)
		setHostField(fields, "host", existing.HostName)
		setHostField(fields, "user", existing.User)
		setHostField(fields, "key", existing.IdentityFile)
		setHostField(fields, "ns", existing.Namespace)
		original = existing.Name
	}

	ti := textinput.New()
	ti.CharLimit = 256
	ti.Width = 56
	ti.Focus()

	m := HostFormScreen{
		store:    store,
		isEdit:   existing != nil,
		original: original,
		fields:   fields,
		input:    ti,
	}
	m.syncInput()
	return m
}

func setHostField(fs []hostField, key, val string) {
	for i := range fs {
		if fs[i].key == key {
			fs[i].value = val
		}
	}
}

func (m HostFormScreen) get(key string) string {
	for _, f := range m.fields {
		if f.key == key {
			return f.value
		}
	}
	return ""
}

func (m *HostFormScreen) syncInput() {
	f := m.fields[m.idx]
	m.input.SetValue(f.value)
	m.input.Placeholder = f.hint
	m.input.CursorEnd()
}

func (m HostFormScreen) Init() tea.Cmd { return textinput.Blink }

func (m HostFormScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch k.String() {
	case "esc":
		return m, pop()
	case "shift+tab", "up":
		if m.idx > 0 {
			m.fields[m.idx].value = m.input.Value()
			m.idx--
			m.syncInput()
		}
		return m, nil
	case "tab", "down":
		m.fields[m.idx].value = m.input.Value()
		if m.idx < len(m.fields)-1 {
			m.idx++
			m.syncInput()
		}
		return m, nil
	case "enter":
		m.fields[m.idx].value = m.input.Value()
		if m.idx < len(m.fields)-1 {
			m.idx++
			m.syncInput()
			return m, nil
		}
		// 最後一欄 enter = save
		if err := m.save(); err != nil {
			m.err = err
			return m, nil
		}
		return m, pop()
	case "ctrl+u":
		m.input.SetValue("")
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m HostFormScreen) save() error {
	name := strings.TrimSpace(m.get("name"))
	host := strings.TrimSpace(m.get("host"))
	if name == "" {
		return fmt.Errorf("Name is required")
	}
	if host == "" {
		return fmt.Errorf("HostName is required")
	}

	// 新增：撞名要擋；編輯：若改名要先刪舊。
	if m.isEdit && m.original != "" && m.original != name {
		_ = m.store.DeleteHost(m.original)
	} else if !m.isEdit {
		if _, exists, _ := m.store.GetHost(name); exists {
			return fmt.Errorf("host %q already exists", name)
		}
	}

	// 保留原本可能已偵測過的 Helm 參數（編輯模式下）
	var helm config.HelmParams
	if m.isEdit {
		if h, ok, _ := m.store.GetHost(m.original); ok {
			helm = h.Helm
		}
	}

	return m.store.PutHost(config.Host{
		Name:         name,
		HostName:     host,
		User:         strings.TrimSpace(m.get("user")),
		IdentityFile: strings.TrimSpace(m.get("key")),
		Namespace:    strings.TrimSpace(m.get("ns")),
		Helm:         helm,
	})
}

func (m HostFormScreen) View() string {
	title := "New Host"
	if m.isEdit {
		title = "Edit Host · " + m.original
	}
	header := titleStyle.Render(title) +
		dimStyle.Render(fmt.Sprintf("   field %d/%d", m.idx+1, len(m.fields)))

	var rows []string
	for i, f := range m.fields {
		label := labelStyle.Render(fmt.Sprintf("  %-14s", f.label))
		if i == m.idx {
			rows = append(rows, valStyle.Render("> ")+label+" "+m.input.View())
		} else {
			val := f.value
			disp := valStyle.Render(val)
			if val == "" {
				disp = dimStyle.Render("(empty)")
			}
			rows = append(rows, "  "+label+" "+disp)
		}
	}
	body := strings.Join(rows, "\n")

	hint := dimStyle.Render("  " + m.fields[m.idx].hint)
	var errLine string
	if m.err != nil {
		errLine = "\n\n" + errStyle.Render("✗ "+m.err.Error())
	}

	saveHint := dimStyle.Render("(last field: enter = save)")
	if m.idx < len(m.fields)-1 {
		saveHint = dimStyle.Render("(enter advances; on the last field it saves)")
	}

	footer := footerStyle.Render("enter next/save · tab/↓ next · shift+tab/↑ back · ctrl+u clear · esc cancel")
	return lipgloss.JoinVertical(lipgloss.Left, header, "", body, "", hint, "  "+saveHint, errLine, "", footer)
}
