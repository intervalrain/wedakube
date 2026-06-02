package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/intervalrain/wedakube/internal/cluster"
	"github.com/intervalrain/wedakube/internal/config"
)

// SetImage 更新既有 deployment 的 image（OTA / 版本 bump）。
func SetImage(ctx context.Context, ssh *cluster.SSH, t config.Target, tag string) error {
	cmd := fmt.Sprintf("kubectl -n %s set image deploy/%s %s=%s:%s",
		t.Namespace, t.Service, t.Service, t.ImageRepo, tag)
	_, err := ssh.Run(ctx, cmd)
	return err
}

type deployStatus struct {
	Metadata struct {
		Generation int `json:"generation"`
	} `json:"metadata"`
	Status struct {
		Replicas           int `json:"replicas"`
		UpdatedReplicas    int `json:"updatedReplicas"`
		ReadyReplicas      int `json:"readyReplicas"`
		AvailableReplicas  int `json:"availableReplicas"`
		ObservedGeneration int `json:"observedGeneration"`
	} `json:"status"`
}

// WaitRollout 輪詢 deployment 狀態驅動進度，直到全部 ready 或 ctx timeout。
func WaitRollout(ctx context.Context, ssh *cluster.SSH, t config.Target, emit Emitter) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	lastReady, lastWant := 0, 1
	for {
		out, err := ssh.Run(ctx, fmt.Sprintf("kubectl -n %s get deploy/%s -o json", t.Namespace, t.Service))
		if err != nil {
			if ctx.Err() != nil {
				return fmt.Errorf("rollout timed out at ready %d/%d — pod likely Running but not Ready (readiness probe / missing dependency)", lastReady, lastWant)
			}
			return err
		}
		var ds deployStatus
		if err := json.Unmarshal(out, &ds); err != nil {
			return fmt.Errorf("parse deploy status: %w", err)
		}

		want := ds.Status.Replicas
		if want == 0 {
			want = 1
		}
		lastReady, lastWant = ds.Status.ReadyReplicas, want
		pct := float64(ds.Status.ReadyReplicas) / float64(want)
		emit(Event{
			Phase: "rollout",
			Msg:   fmt.Sprintf("ready %d/%d  updated %d", ds.Status.ReadyReplicas, want, ds.Status.UpdatedReplicas),
			Pct:   pct,
		})

		if ds.Status.ObservedGeneration >= ds.Metadata.Generation &&
			ds.Status.UpdatedReplicas == want &&
			ds.Status.ReadyReplicas == want &&
			ds.Status.AvailableReplicas == want {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("rollout timed out at ready %d/%d — pod likely Running but not Ready (readiness probe / missing dependency)", lastReady, lastWant)
		case <-ticker.C:
		}
	}
}

// Diagnose 在 rollout 失敗時抓 describe + logs（保留證據）。
// 先抓當前 logs（pod 卡住但沒重啟時才看得到原因），previous 只在有重啟時補充。
func Diagnose(ctx context.Context, ssh *cluster.SSH, t config.Target) string {
	desc, _ := ssh.Run(ctx, fmt.Sprintf("kubectl -n %s describe pod -l app=%s | tail -n 40", t.Namespace, t.Service))
	cur, _ := ssh.Run(ctx, fmt.Sprintf("kubectl -n %s logs deploy/%s --tail=60 2>&1", t.Namespace, t.Service))

	out := string(desc) + "\n--- current logs ---\n" + string(cur)

	prev, _ := ssh.Run(ctx, fmt.Sprintf("kubectl -n %s logs deploy/%s --previous --tail=60 2>/dev/null", t.Namespace, t.Service))
	if len(strings.TrimSpace(string(prev))) > 0 {
		out += "\n--- previous (crashed) logs ---\n" + string(prev)
	}
	return out
}

// Rollback 退回上一個 revision。
func Rollback(ctx context.Context, ssh *cluster.SSH, t config.Target) error {
	_, err := ssh.Run(ctx, fmt.Sprintf("kubectl -n %s rollout undo deploy/%s", t.Namespace, t.Service))
	return err
}
