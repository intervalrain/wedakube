package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/intervalrain/wedakube/internal/cluster"
	"github.com/intervalrain/wedakube/internal/config"
)

// HelmsterDeploy 走 infra team 的標準路徑：
//   scp appcfg.yaml -> helmster gen -> helm upgrade --install
// 這條路徑會建/更新 Application CRD 讓 ECO 接管 broker/creds，
// 是唯一能讓需要 ECO 的服務真正 Ready 的方式。
func HelmsterDeploy(ctx context.Context, ssh *cluster.SSH, t config.Target, hp config.HelmParams, tag string, emit Emitter) error {
	svc := t.Service

	// 解析 node 家目錄，取得 opsmanager 絕對路徑（scp 不展開 $HOME）。
	homeOut, err := ssh.Run(ctx, "echo $HOME")
	if err != nil {
		return fmt.Errorf("resolve $HOME: %w", err)
	}
	home := strings.TrimSpace(string(homeOut))
	remoteDir := filepath.Join(home, ".opsmanager/playbooks/roles/service", svc)
	remoteFiles := filepath.Join(remoteDir, "files")

	// Like CI, pin appcfg's version to this build's tag before helmster gen, so the chart's
	// pre-upgrade Jobs (db-migrator) pull the image just pushed, not the committed tag.
	appcfg, err := os.ReadFile(filepath.Join(t.Context, "appcfg.yaml"))
	if err != nil {
		return fmt.Errorf("read appcfg.yaml: %w", err)
	}
	tmp, err := os.CreateTemp("", "appcfg-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(withAppcfgVersion(appcfg, tag)); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	emit(Event{Phase: "scp", Msg: "appcfg.yaml (version " + tag + ") -> " + remoteFiles, Pct: -1})
	if _, err := ssh.Run(ctx, "mkdir -p "+remoteFiles); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	if err := ssh.ScpUpload(ctx, tmp.Name(), filepath.Join(remoteFiles, "appcfg.yaml")); err != nil {
		return err
	}

	emit(Event{Phase: "helmster", Msg: "gen chart", Pct: -1})
	if out, err := ssh.Run(ctx, "cd "+remoteDir+" && helmster gen -f files/appcfg.yaml -o ./output 2>&1"); err != nil {
		return fmt.Errorf("helmster: %w: %s", err, string(out))
	}

	ns := t.Namespace
	if ns == "" {
		ns = hp.TenantID + "-" + hp.SrpName
	}
	imageRepo := basenameOf(t.ImageRepo)

	emit(Event{Phase: "helm-upgrade", Msg: "helm upgrade --install " + svc + " -n " + ns, Pct: -1})
	cmd := strings.Join([]string{
		"cd " + remoteDir + " && helm upgrade --install " + svc + " ./output/" + svc,
		"-n " + ns,
		"--create-namespace",
		serviceTypeFlag(ctx, ssh, ns, svc),
		setQ("global.repo.registry", hp.Registry),
		setQ("global.repo.project", hp.Project),
		setQ("global.domain", hp.Domain),
		"--set global.dbProvider=postgresql",
		setQ("global.tenantId", hp.TenantID),
		setQ("global.tenantPath", hp.TenantPath),
		setQ("global.tenantAlias", hp.TenantAlias),
		setQ("global.srpName", hp.SrpName),
		"--set global.eco.enabled=true",
		"--set global.eco.crd=true",
		setQ("global.eco.endpoint", hp.EcoEndpoint),
		setQ("global.eco.apiKey", hp.EcoApiKey),
		setQ("image.repository", imageRepo),
		setQ("image.tag", tag),
		"2>&1",
	}, " ")
	out, err := ssh.Run(ctx, cmd)
	if err != nil {
		// The command carries global.eco.apiKey; never echo it, and mask it in helm's output.
		return fmt.Errorf("helm upgrade %s -n %s: %w: %s", svc, ns, redactErr(err, hp.EcoApiKey), redact(string(out), hp.EcoApiKey))
	}
	// 取 STATUS / REVISION 行做簡短回報
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "STATUS:") || strings.HasPrefix(line, "REVISION:") {
			emit(Event{Phase: "helm-upgrade", Msg: line, Pct: -1})
		}
	}
	return nil
}

// HelmsterAvailable 檢查 node 上有沒有 helmster 執行檔。
// 不是每台 node 都裝了（dev node 常常只有 helm），沒有就得走 OCI chart fallback。
func HelmsterAvailable(ctx context.Context, ssh *cluster.SSH) bool {
	out, err := ssh.Run(ctx, "command -v helmster >/dev/null 2>&1 && echo yes || echo no")
	return err == nil && strings.TrimSpace(string(out)) == "yes"
}

// HelmChartUpgrade 是沒有 helmster 時的 fallback：直接拿 registry 上現成的 OCI chart，
// --reuse-values 沿用既有 release 的設定，只換 image.tag。
// 因為不重跑 helmster gen，appcfg.yaml 的變更不會生效——呼叫端要提醒使用者。
func HelmChartUpgrade(ctx context.Context, ssh *cluster.SSH, t config.Target, hp config.HelmParams, release, ns, tag string, emit Emitter) error {
	chartVer, err := helmChartVersion(ctx, ssh, release, ns)
	if err != nil {
		return fmt.Errorf("resolve chart version for %s: %w", release, err)
	}
	chart := fmt.Sprintf("oci://%s/%s/charts/%s", hp.Registry, hp.Project, basenameOf(t.ImageRepo))

	emit(Event{Phase: "helm-upgrade", Msg: "no helmster on node — OCI chart " + chart + ":" + chartVer + " (appcfg.yaml changes NOT applied)", Pct: -1})
	cmd := strings.Join([]string{
		"helm upgrade " + release + " " + chart,
		"--version " + chartVer,
		"-n " + ns,
		"--reuse-values",
		serviceTypeFlag(ctx, ssh, ns, t.Service),
		setQ("image.tag", tag),
		"2>&1",
	}, " ")
	out, err := ssh.Run(ctx, cmd)
	if err != nil {
		return fmt.Errorf("helm: %w: %s", err, string(out))
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "STATUS:") || strings.HasPrefix(line, "REVISION:") {
			emit(Event{Phase: "helm-upgrade", Msg: line, Pct: -1})
		}
	}
	return nil
}

// helmChartVersion 從 helm list 取得 release 目前用的 chart 版本。
// chart 欄位長得像 "device-shadows-1.1.1-20260724.1"，去掉 "<name>-" 前綴就是版本。
func helmChartVersion(ctx context.Context, ssh *cluster.SSH, release, ns string) (string, error) {
	out, err := ssh.Run(ctx, fmt.Sprintf("helm list -n %s -o json --filter '^%s$'", ns, release))
	if err != nil {
		return "", err
	}
	return parseChartVersion(out, release)
}

func parseChartVersion(out []byte, release string) (string, error) {
	var rows []struct {
		Name  string `json:"name"`
		Chart string `json:"chart"`
	}
	if err := json.Unmarshal(out, &rows); err != nil {
		return "", err
	}
	for _, r := range rows {
		if r.Name == release && r.Chart != "" {
			if v := strings.TrimPrefix(r.Chart, r.Name+"-"); v != r.Chart {
				return v, nil
			}
		}
	}
	return "", fmt.Errorf("release %q not found in helm list", release)
}

// serviceTypeFlag 把 Service 現在的 .spec.type 明講給 helm。
// 有人用 kubectl patch 動過 type（例如開 NodePort 看 swagger）之後，
// helm 的 server-side apply 會跟 kubectl-patch 這個 field manager 撞成 conflict；
// 帶入相同的值就變成共同持有，不會衝突，也保住既有的 nodePort。
func serviceTypeFlag(ctx context.Context, ssh *cluster.SSH, ns, svc string) string {
	out, err := ssh.Run(ctx, fmt.Sprintf("kubectl -n %s get svc %s -o jsonpath='{.spec.type}' 2>/dev/null", ns, svc))
	if err != nil {
		return ""
	}
	st := strings.TrimSpace(string(out))
	if st == "" {
		return ""
	}
	return setQ("service.type", st)
}

// HelmRollback 退一個 release 的版本（取代 M2 的 kubectl rollout undo）。
func HelmRollback(ctx context.Context, ssh *cluster.SSH, release, ns string) error {
	_, err := ssh.Run(ctx, fmt.Sprintf("helm rollback %s -n %s 2>&1", release, ns))
	return err
}

func setQ(k, v string) string {
	// 值含特殊字元時用單引號包起來，shell 解析才安全。
	return "--set " + k + "='" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}

func basenameOf(imageRepo string) string {
	if i := strings.LastIndex(imageRepo, "/"); i >= 0 {
		return imageRepo[i+1:]
	}
	return imageRepo
}

// redactErr masks secrets in an error's text (ssh errors can quote the remote command).
func redactErr(err error, secrets ...string) error {
	if err == nil {
		return nil
	}
	return errors.New(redact(err.Error(), secrets...))
}
