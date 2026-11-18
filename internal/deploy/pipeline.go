package deploy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/intervalrain/wedakube/internal/cluster"
	"github.com/intervalrain/wedakube/internal/config"
)

const rolloutTimeout = 3 * time.Minute

// Deploy 跑完整流程：算 tag → buildx+push → (首次 apply / 既有 set image) → 等 rollout。
// rollout 失敗時抓 logs，若是既有服務則自動 rollback。回傳這次用的 tag。
// ref picks the commit to build ("" = HEAD). The build context is a clean export of that
// commit plus its submodules (see ExportSource), never the working tree, like CI.
func Deploy(ctx context.Context, ssh *cluster.SSH, store *config.Store, t config.Target, hp config.HelmParams, date, feedPAT, ref string, rollbackOnFail bool, emit Emitter) (string, error) {
	n, err := store.NextBuildNumber(t.Service, date)
	if err != nil {
		return "", err
	}
	tag := BuildTag(t.VersionBase, date, n)
	emit(Event{Phase: "tag", Msg: tag, Pct: -1})

	// build 前先偵測目標 node 的 os/arch，讓 --platform 跟著 node 走（amd64 / arm64 都通）。
	platform, perr := cluster.NodePlatform(ctx, ssh)
	if perr != nil {
		platform = DefaultPlatform
		emit(Event{Phase: "platform", Msg: fmt.Sprintf("detect failed (%v), fallback %s", perr, platform), Pct: -1})
	} else {
		emit(Event{Phase: "platform", Msg: platform, Pct: -1})
	}

	src, err := ExportSource(ctx, t.Context, ref)
	if err != nil {
		return tag, fmt.Errorf("source: %w", err)
	}
	defer src.Cleanup()
	label := ref
	if label == "" {
		label = "HEAD"
	}
	emit(Event{Phase: "source", Msg: fmt.Sprintf("%s @ %s (clean export, submodules at pinned commits)", label, short(src.Rev)), Pct: -1})
	if src.Dirty {
		emit(Event{Phase: "source", Msg: "warning: the working tree has uncommitted changes; they are NOT in this build — commit them to ship them", Pct: -1})
	}
	if t, err = onExport(t, src); err != nil {
		return tag, fmt.Errorf("source: %w", err)
	}
	if n, err := InjectFeedPAT(src.Dir, feedPAT); err != nil {
		return tag, fmt.Errorf("inject feed PAT: %w", err)
	} else if n > 0 {
		emit(Event{Phase: "source", Msg: fmt.Sprintf("feed PAT injected into %d NuGet.Config (export only)", n), Pct: -1})
	}

	if err := BuildAndPush(ctx, t, tag, platform, feedPAT, emit); err != nil {
		return tag, fmt.Errorf("build: %w", err)
	}

	// 路徑切換：repo 有 appcfg.yaml OR cluster 已是 helm release → 走 helmster
	useHelmster := false
	if _, err := os.Stat(filepath.Join(t.Context, "appcfg.yaml")); err == nil {
		useHelmster = true
	}
	kc := cluster.NewKubectl(ssh, t.Namespace)
	existingRelease, releaseNS, _ := kc.HelmReleaseFor(ctx, t.Service)
	if existingRelease != "" {
		useHelmster = true
	}

	if useHelmster {
		if hp.TenantID == "" {
			return tag, fmt.Errorf("helmster path needs host Helm params (tenantId/srp/eco…) — connect host first to auto-detect")
		}
		// 若該服務已有 release，強制用 release 所在 ns，避免在錯的 ns 開孤兒。
		if existingRelease != "" && releaseNS != "" {
			t.Namespace = releaseNS
		}
		switch {
		case HelmsterAvailable(ctx, ssh):
			if err := HelmsterDeploy(ctx, ssh, t, hp, tag, emit); err != nil {
				return tag, err
			}
		case existingRelease != "":
			// node 沒裝 helmster，但服務已經是 helm release → 用 registry 上的 OCI chart 換 tag。
			if err := HelmChartUpgrade(ctx, ssh, t, hp, existingRelease, releaseNS, tag, emit); err != nil {
				return tag, err
			}
		default:
			return tag, fmt.Errorf("no helmster on node and no existing helm release for %s — "+
				"install helmster on the node, or deploy the service once via the infra pipeline first", t.Service)
		}
	} else {
		exists := DeploymentExists(ctx, ssh, t)
		if exists {
			emit(Event{Phase: "setimage", Msg: tag, Pct: -1})
			if err := SetImage(ctx, ssh, t, tag); err != nil {
				return tag, err
			}
		} else {
			emit(Event{Phase: "apply", Msg: "creating " + t.Service, Pct: -1})
			if err := EnsureNamespace(ctx, ssh, t.Namespace); err != nil {
				return tag, err
			}
			if err := Apply(ctx, ssh, []byte(Render(t, tag))); err != nil {
				return tag, err
			}
		}
	}

	rctx, cancel := context.WithTimeout(ctx, rolloutTimeout)
	defer cancel()
	if err := WaitRollout(rctx, ssh, t, emit); err != nil {
		emit(Event{Phase: "fail", Msg: Diagnose(ctx, ssh, t), Pct: -1})
		if rollbackOnFail {
			switch {
			case useHelmster && existingRelease != "":
				emit(Event{Phase: "rollback", Msg: "helm rollback " + existingRelease, Pct: -1})
				if rbErr := HelmRollback(ctx, ssh, existingRelease, releaseNS); rbErr != nil {
					return tag, fmt.Errorf("rollout failed: %w; helm rollback also failed: %v", err, rbErr)
				}
			case !useHelmster && DeploymentExists(ctx, ssh, t):
				emit(Event{Phase: "rollback", Msg: "rolling back to previous revision", Pct: -1})
				if rbErr := Rollback(ctx, ssh, t); rbErr != nil {
					return tag, fmt.Errorf("rollout failed: %w; rollback also failed: %v", err, rbErr)
				}
			}
		}
		return tag, fmt.Errorf("rollout failed: %w", err)
	}

	emit(Event{Phase: "done", Msg: tag, Pct: 1})
	return tag, nil
}

// onExport points the target's Dockerfile and build context at the export instead of the
// working tree. Paths are compared after resolving symlinks (/tmp vs /private/tmp on macOS).
func onExport(t config.Target, src Source) (config.Target, error) {
	root := resolve(src.Root)
	ctxDir, err := rebase(resolve(t.Context), root, src.Dir)
	if err != nil {
		return t, err
	}
	dockerfile := t.Dockerfile
	if dockerfile == "" {
		dockerfile = filepath.Join(t.Context, "Dockerfile")
	} else if !filepath.IsAbs(dockerfile) {
		dockerfile = filepath.Join(t.Context, dockerfile)
	}
	df, err := rebase(resolve(dockerfile), root, src.Dir)
	if err != nil {
		return t, err
	}
	t.Context, t.Dockerfile = ctxDir, df
	return t, nil
}

func resolve(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}
