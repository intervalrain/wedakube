package deploy

import (
	"context"
	"fmt"
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

	emit(Event{Phase: "scp", Msg: "appcfg.yaml -> " + remoteFiles, Pct: -1})
	if _, err := ssh.Run(ctx, "mkdir -p "+remoteFiles); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	if err := ssh.ScpUpload(ctx, filepath.Join(t.Context, "appcfg.yaml"), filepath.Join(remoteFiles, "appcfg.yaml")); err != nil {
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
		return fmt.Errorf("helm: %w: %s", err, string(out))
	}
	// 取 STATUS / REVISION 行做簡短回報
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "STATUS:") || strings.HasPrefix(line, "REVISION:") {
			emit(Event{Phase: "helm-upgrade", Msg: line, Pct: -1})
		}
	}
	return nil
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
