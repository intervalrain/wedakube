package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/intervalrain/wedakube/internal/config"
)

// GuideScreen 是引導模式：把 TUI 每個動作背後的真實指令攤開來，
// 填上目前 host / namespace / service 的實際值，讓使用者可以直接複製到終端機自己跑。
type GuideScreen struct {
	title string
	body  string
	vp    viewport.Model
}

func NewGuide(title, body string) GuideScreen {
	vp := viewport.New(100, 24)
	vp.SetContent(body)
	return GuideScreen{title: title, body: body, vp: vp}
}

func (m GuideScreen) Init() tea.Cmd { return nil }

func (m GuideScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.vp.Width = msg.Width
		m.vp.Height = msg.Height - 3
		m.vp.SetContent(m.body)
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q", "g":
			return m, pop()
		}
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

func (m GuideScreen) View() string {
	header := titleStyle.Render(m.title) + dimStyle.Render("  copy & run these yourself")
	footer := footerStyle.Render("↑/↓ scroll · esc back")
	return lipgloss.JoinVertical(lipgloss.Left, header, m.vp.View(), footer)
}

// ---------- content builders ----------

// sshLine 組出登入這台 host 的 ssh 指令。
func sshLine(h config.Host) string {
	var b strings.Builder
	b.WriteString("ssh")
	if h.IdentityFile != "" {
		b.WriteString(" -i " + h.IdentityFile)
	}
	if h.User != "" {
		b.WriteString(" " + h.User + "@" + h.Dest())
	} else {
		b.WriteString(" " + h.Dest())
	}
	return b.String()
}

func section(title string, lines ...string) string {
	return "── " + title + " " + strings.Repeat("─", max(0, 56-len(title))) + "\n" + strings.Join(lines, "\n") + "\n"
}

// HostGuideContent 是 L2（service list）層級的指令手冊。
func HostGuideContent(h config.Host, ns string) string {
	srp := h.Helm.SrpName
	if srp == "" {
		srp = "weda"
	}
	apiKey := h.Helm.EcoApiKey
	if apiKey == "" {
		apiKey = "<ECO_API_KEY: 按 enter 連一次 host 就會自動偵測>"
	}

	var b strings.Builder
	b.WriteString(section("connect",
		"# 登入 node",
		sshLine(h),
	))
	b.WriteString("\n" + section("discover",
		"# 找 tenant namespace（結尾 -"+srp+"）",
		"kubectl get ns | grep -- -"+srp,
		"",
		"# 看這個 ns 部署了哪些服務",
		"kubectl -n "+ns+" get deploy",
		"",
		"# 看某個服務用的 image 與 ECO 設定",
		"kubectl -n "+ns+" get deploy <svc> -o jsonpath='{.spec.template.spec.containers[0].env}' | tr ',' '\\n' | grep ECO",
	))
	b.WriteString("\n" + section("eco api（在 node 上執行）",
		"# 1. 解析 eco-server 的 clusterIP（node 上 svc DNS 不通，用 IP 直打）",
		"ECO=http://$(kubectl -n wise-system get svc eco-server -o jsonpath='{.spec.clusterIP}'):7777",
		"",
		"# 2. 用 API key 換 token（注意：回傳欄位是 accessToken）",
		"TOKEN=$(curl -s -X POST $ECO/auth/token \\",
		"  -H 'Content-Type: application/json' \\",
		"  -d '{\"apiKey\":\""+apiKey+"\",\"srpName\":\""+srp+"\"}' \\",
		"  | python3 -c \"import sys,json; print(json.load(sys.stdin)['accessToken'])\")",
		"",
		"# 3. 列出所有 registered app",
		"curl -s $ECO/v1/applications -H \"Authorization: Bearer $TOKEN\" | python3 -m json.tool",
		"",
		"# 4. 查某個 app（拿 id）",
		"curl -s \"$ECO/v1/applications?name=<svc>\" -H \"Authorization: Bearer $TOKEN\" | python3 -m json.tool",
		"",
		"# 5. 用 id 抓完整 config",
		"curl -s $ECO/v1/applications/<id> -H \"Authorization: Bearer $TOKEN\" | python3 -m json.tool",
		"",
		"# 6. 更新 config（把編輯後的 JSON PUT 回去）",
		"curl -s -X PUT $ECO/v1/applications/<id> -H \"Authorization: Bearer $TOKEN\" \\",
		"  -H 'Content-Type: application/json' -d @edited.json",
	))
	b.WriteString("\n" + section("troubleshoot",
		"# rollout 卡在 ready 1/2 → 新 pod 起不來，看 crash 原因",
		"kubectl -n "+ns+" get pod | grep <svc>",
		"kubectl -n "+ns+" logs <pod> --previous --tail=50",
		"",
		"# ECO login 404 = app 沒 register 或 key 不對；比對 deployment 的 key 與 ECO 的 key",
		"# ECO 'record not found' = 這組 apiKey 不在 ECO DB 裡",
	))
	return b.String()
}

// ServiceGuideContent 是 L3（單一服務）層級的指令手冊。
func ServiceGuideContent(h config.Host, ns, svc string, t *config.Target) string {
	hp := h.Helm
	srp := hp.SrpName
	if srp == "" {
		srp = "weda"
	}

	var b strings.Builder
	b.WriteString(section("inspect",
		"kubectl -n "+ns+" get deploy,rs,pod -l app.kubernetes.io/name="+svc+" -o wide",
		"kubectl -n "+ns+" describe deploy "+svc,
		"kubectl -n "+ns+" logs deploy/"+svc+" -f --tail=100",
		"kubectl -n "+ns+" logs deploy/"+svc+" --previous --tail=50   # crash 的上一世 log",
	))
	b.WriteString("\n" + section("lifecycle",
		"kubectl -n "+ns+" rollout restart deploy/"+svc,
		"kubectl -n "+ns+" scale deploy/"+svc+" --replicas=0   # stop",
		"kubectl -n "+ns+" scale deploy/"+svc+" --replicas=1   # start",
		"kubectl -n "+ns+" rollout undo deploy/"+svc+"          # 非 helm 的 rollback",
		"helm rollback "+svc+" -n "+ns+"                 # helm 的 rollback",
		"helm history "+svc+" -n "+ns,
	))

	if t != nil {
		image := t.ImageRepo + ":" + t.VersionBase + "_<YYYYMMDD>.<N>"
		b.WriteString("\n" + section("build & push（本機執行）",
			"# 本機 arm64 → node amd64，--platform 不可省",
			"docker buildx build --platform linux/amd64 \\",
			"  -f "+t.Dockerfile+" \\",
			"  -t "+image+" \\",
			"  --push "+t.Context,
		))
	}

	registry, project := hp.Registry, hp.Project
	if registry == "" {
		registry = "<registry>"
	}
	if project == "" {
		project = "<project>"
	}
	ecoKey := hp.EcoApiKey
	if ecoKey == "" {
		ecoKey = "<ECO_API_KEY>"
	}
	b.WriteString("\n" + section("helm 部署（node 上執行，OCI chart 路徑）",
		"helm upgrade --install "+svc+" \\",
		"  oci://"+registry+"/"+project+"/charts/"+svc+" \\",
		"  --version <chart-version> \\",
		"  --namespace "+ns+" --create-namespace \\",
		"  --set image.registry="+registry+" \\",
		"  --set image.project="+project+" \\",
		"  --set image.repository="+svc+" \\",
		"  --set image.tag=<image-tag> \\",
		"  --set global.repo.registry="+registry+" \\",
		"  --set global.repo.project="+project+" \\",
		"  --set global.domain="+orPlaceholder(hp.Domain, "<domain>")+" \\",
		"  --set global.dbProvider=postgresql \\",
		"  --set global.tenantId="+orPlaceholder(hp.TenantID, "<tenantId>")+" \\",
		"  --set global.tenantPath="+orPlaceholder(hp.TenantPath, "<tenantPath>")+" \\",
		"  --set global.tenantAlias="+orPlaceholder(hp.TenantAlias, "<tenantAlias>")+" \\",
		"  --set global.srpName="+srp+" \\",
		"  --set global.eco.enabled=true --set global.eco.crd=true \\",
		"  --set global.eco.endpoint="+orPlaceholder(hp.EcoEndpoint, "http://eco-server.wise-system.svc.cluster.local:7777")+" \\",
		"  --set global.eco.apiKey='"+ecoKey+"'",
		"",
		"# 只換 image tag（沿用其他 values）",
		"helm upgrade "+svc+" oci://"+registry+"/"+project+"/charts/"+svc+" \\",
		"  --version <chart-version> -n "+ns+" \\",
		"  --reuse-values --set image.tag=<new-tag>",
	))
	b.WriteString("\n" + section("watch rollout",
		"kubectl -n "+ns+" rollout status deploy/"+svc+" --timeout=180s",
		"kubectl -n "+ns+" get pod -l app.kubernetes.io/name="+svc+" -w",
	))
	b.WriteString("\n" + section("eco config（node 上執行）",
		"ECO=http://$(kubectl -n wise-system get svc eco-server -o jsonpath='{.spec.clusterIP}'):7777",
		"TOKEN=$(curl -s -X POST $ECO/auth/token -H 'Content-Type: application/json' \\",
		"  -d '{\"apiKey\":\""+ecoKey+"\",\"srpName\":\""+srp+"\"}' \\",
		"  | python3 -c \"import sys,json; print(json.load(sys.stdin)['accessToken'])\")",
		"ID=$(curl -s \"$ECO/v1/applications?name="+svc+"\" -H \"Authorization: Bearer $TOKEN\" \\",
		"  | python3 -c \"import sys,json; print(json.load(sys.stdin)['data'][0]['id'])\")",
		"curl -s $ECO/v1/applications/$ID -H \"Authorization: Bearer $TOKEN\" | python3 -m json.tool",
	))
	return b.String()
}

func orPlaceholder(v, ph string) string {
	if v == "" {
		return ph
	}
	return v
}
