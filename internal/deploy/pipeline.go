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
func Deploy(ctx context.Context, ssh *cluster.SSH, store *config.Store, t config.Target, hp config.HelmParams, date, feedPAT string, rollbackOnFail bool, emit Emitter) (string, error) {
	n, err := store.NextBuildNumber(t.Service, date)
	if err != nil {
		return "", err
	}
	tag := BuildTag(t.VersionBase, date, n)
	emit(Event{Phase: "tag", Msg: tag, Pct: -1})

	if err := BuildAndPush(ctx, t, tag, feedPAT, emit); err != nil {
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
		if err := HelmsterDeploy(ctx, ssh, t, hp, tag, emit); err != nil {
			return tag, err
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
