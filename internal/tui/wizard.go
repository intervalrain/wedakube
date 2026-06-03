package tui

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/intervalrain/wedakube/internal/cluster"
	"github.com/intervalrain/wedakube/internal/config"
)

type wizField struct{ key, label, hint, value string }

// WizardScreen 引導使用者一步步建立一個部署目標（新服務），最後存檔並可直接部署。
type WizardScreen struct {
	store  *config.Store
	ssh    *cluster.SSH
	host   string
	fields []wizField
	idx    int // 0..len(fields)；== len(fields) 為 review 步驟
	input  textinput.Model
}

func NewWizard(store *config.Store, ssh *cluster.SSH, host string) WizardScreen {
	// 預設 namespace = host 推導出的 SRP namespace（無則留空，待 detect/手填）
	defaultNS := ""
	if h, ok, _ := store.GetHost(host); ok {
		defaultNS = h.DerivedNamespace()
	}
	fields := []wizField{
		{key: "repo", label: "Repo path", hint: "~/path/to/your/service"},
		{key: "service", label: "Service name", hint: "k8s deployment name"},
		{key: "image", label: "Image repo", hint: "registry/project/name"},
		{key: "ns", label: "Namespace", hint: "(host's SRP ns)", value: defaultNS},
		{key: "docker", label: "Dockerfile", hint: "<repo>/Dockerfile"},
		{key: "port", label: "Container port", hint: "5001"},
		{key: "version", label: "Version base", hint: "v1.1.0"},
	}
	ti := textinput.New()
	ti.CharLimit = 200
	ti.Width = 54
	ti.Focus()

	w := WizardScreen{store: store, ssh: ssh, host: host, fields: fields, input: ti}
	w.syncInput()
	return w
}

func (m WizardScreen) Init() tea.Cmd { return textinput.Blink }

func (m *WizardScreen) syncInput() {
	if m.idx < len(m.fields) {
		f := m.fields[m.idx]
		m.input.SetValue(f.value)
		m.input.Placeholder = f.hint
		m.input.CursorEnd()
	}
}

func (m WizardScreen) get(key string) string {
	for _, f := range m.fields {
		if f.key == key {
			return f.value
		}
	}
	return ""
}

func (m *WizardScreen) set(key, val string) {
	for i := range m.fields {
		if m.fields[i].key == key {
			m.fields[i].value = val
		}
	}
}

// detect 在輸入完 repo 後，從 appcfg.yaml + Host.Helm 推 service/image/version/port 的預設值。
func (m *WizardScreen) detect(repo string) {
	repo = expandHome(repo)

	// 1) 從 repo/appcfg.yaml 抓 name / version（WEDA 標準）
	cfgName, cfgVersion := parseAppcfg(repo)

	// 2) service 名：appcfg.name 優先，否則用 repo 資料夾名
	svc := cfgName
	if svc == "" {
		svc = sanitizeName(filepath.Base(strings.TrimRight(repo, "/")))
	}
	if m.get("service") == "" {
		m.set("service", svc)
	}

	// 3) Dockerfile 與 port
	df := filepath.Join(repo, "Dockerfile")
	port := 8080
	if b, err := os.ReadFile(df); err == nil {
		port = detectPort(string(b), port)
		if m.get("docker") == "" {
			m.set("docker", df)
		}
	}
	if m.get("port") == "" {
		m.set("port", strconv.Itoa(port))
	}

	// 4) image repo：用 host 的 registry/project（自動偵測過）+ service 原名（不轉底線）
	if m.get("image") == "" {
		registry, project := "registry.example.com", "edge-coa"
		if h, ok, _ := m.store.GetHost(m.host); ok {
			if h.Helm.Registry != "" {
				registry = h.Helm.Registry
			}
			if h.Helm.Project != "" {
				project = h.Helm.Project
			}
		}
		m.set("image", registry+"/"+project+"/"+svc)
	}

	// 5) version base：用 appcfg.version 的 base 段（去掉 _date.N 後綴）
	if m.get("version") == "" {
		if vb := versionBase(cfgVersion); vb != "" {
			m.set("version", vb)
		} else {
			m.set("version", "v0.1.0")
		}
	}
}

// parseAppcfg 從 repo 根的 appcfg.yaml 抓 name / version（用簡單行掃描，免拉 yaml dep）。
func parseAppcfg(repo string) (name, version string) {
	b, err := os.ReadFile(filepath.Join(repo, "appcfg.yaml"))
	if err != nil {
		return "", ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		s := strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(s, "name:"); ok && name == "" {
			name = strings.Trim(strings.TrimSpace(rest), `"'`)
		} else if rest, ok := strings.CutPrefix(s, "version:"); ok && version == "" {
			version = strings.Trim(strings.TrimSpace(rest), `"'`)
		}
	}
	return
}

// versionBase 把 "v0.2.0_20260121.1" 截成 "v0.2.0"，"v1.1.0" 不變。
func versionBase(v string) string {
	if i := strings.IndexAny(v, "_-+"); i > 0 {
		// 保留前綴 v 與 semver 主幹
		base := v[:i]
		if strings.HasPrefix(base, "v") {
			return base
		}
	}
	return v
}

func (m WizardScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	// review 步驟
	if m.idx == len(m.fields) {
		switch k.String() {
		case "esc":
			return m, pop()
		case "shift+tab", "up":
			m.idx--
			m.syncInput()
			return m, nil
		case "enter": // 存檔 + 直接部署
			t := m.buildTarget()
			m.store.PutTarget(t)
			return m, push(NewDeployScreen(m.ssh, m.store, t))
		case "s": // 只存檔
			m.store.PutTarget(m.buildTarget())
			return m, pop()
		}
		return m, nil
	}

	switch k.String() {
	case "esc":
		return m, pop()
	case "enter", "tab":
		m.fields[m.idx].value = strings.TrimSpace(m.input.Value())
		if m.fields[m.idx].key == "repo" {
			m.detect(m.fields[m.idx].value)
		}
		m.idx++
		m.syncInput()
		return m, nil
	case "shift+tab":
		if m.idx > 0 {
			m.fields[m.idx].value = m.input.Value()
			m.idx--
			m.syncInput()
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m WizardScreen) buildTarget() config.Target {
	port, _ := strconv.Atoi(m.get("port"))
	repo := expandHome(m.get("repo"))
	df := m.get("docker")
	if df == "" {
		df = filepath.Join(repo, "Dockerfile")
	}
	return config.Target{
		RepoPath:    repo,
		Host:        m.host,
		Service:     m.get("service"),
		Namespace:   m.get("ns"),
		ImageRepo:   m.get("image"),
		VersionBase: m.get("version"),
		Dockerfile:  df,
		Context:     repo,
		Port:        port,
		SSHAlias:    m.host,
	}
}

func (m WizardScreen) View() string {
	if m.idx == len(m.fields) {
		return m.reviewView()
	}

	header := titleStyle.Render("wizard · new deployment") +
		dimStyle.Render("   step "+strconv.Itoa(m.idx+1)+"/"+strconv.Itoa(len(m.fields)))
	cur := m.fields[m.idx]
	field := valStyle.Render(cur.label) + "\n" + m.input.View() + "\n" + dimStyle.Render("  "+cur.hint)

	// 已填的回顧
	var done []string
	for i := 0; i < m.idx; i++ {
		done = append(done, "  "+labelStyle.Render(pad(m.fields[i].label, 14))+valStyle.Render(m.fields[i].value))
	}
	review := dimStyle.Render(strings.Join(done, "\n"))

	footer := footerStyle.Render("enter next · shift+tab back · esc cancel")
	return lipgloss.JoinVertical(lipgloss.Left, header, "", field, "", review, footer)
}

func (m WizardScreen) reviewView() string {
	header := titleStyle.Render("wizard · review")
	rows := []string{}
	for _, f := range m.fields {
		v := f.value
		if v == "" {
			v = dimStyle.Render("(empty)")
		} else {
			v = valStyle.Render(v)
		}
		rows = append(rows, "  "+labelStyle.Render(pad(f.label, 14))+v)
	}
	body := strings.Join(rows, "\n")
	deploy := lipgloss.NewStyle().Foreground(lipgloss.Color("114")).Bold(true).Render("enter") +
		labelStyle.Render(" save & deploy   ") +
		keyStyle.Render("s") + labelStyle.Render(" save only   ") +
		dimStyle.Render("shift+tab back · esc cancel")
	return lipgloss.JoinVertical(lipgloss.Left, header, "", body, "", deploy)
}

// --- helpers ---

func expandHome(p string) string {
	if strings.HasPrefix(p, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

var nameRe = regexp.MustCompile(`[^a-z0-9-]+`)

func sanitizeName(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "_", "-")
	s = nameRe.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

var portRe = regexp.MustCompile(`(?i)(?:ASPNETCORE_HTTP_PORTS=|EXPOSE\s+)(\d{2,5})`)

func detectPort(dockerfile string, def int) int {
	if m := portRe.FindStringSubmatch(dockerfile); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n
		}
	}
	return def
}

func pad(s string, n int) string {
	for len(s) < n {
		s += " "
	}
	return s
}
