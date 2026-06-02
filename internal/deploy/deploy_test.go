package deploy

import (
	"context"
	"testing"
	"time"

	"github.com/intervalrain/wedakube/internal/cluster"
)

// 用 nginx stand-in 打真 cluster，驗證 cluster 端機制（不需要 FEED_PAT / buildx）。
// 涵蓋：建 ns -> 首次 apply -> rollout -> set image -> rollout -> rollback -> rollout。
func TestRolloutMechanicsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; needs cluster access")
	}

	ssh := cluster.NewSSH("my-cluster")
	defer ssh.Close()

	tgt := Target{
		Service:   "pipetest",
		Namespace: "wedakube-dev",
		ImageRepo: "nginx", // stand-in：nginx:<tag>
		Port:      80,
	}
	emit := func(e Event) { t.Logf("[%s] %s pct=%.2f", e.Phase, e.Msg, e.Pct) }

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	// 收尾：刪掉我們自己的測試資源（policy 禁的是 WEDA 資源，這是 wedakube-dev 測試物）。
	defer func() {
		c, cc := context.WithTimeout(context.Background(), 30*time.Second)
		defer cc()
		ssh.Run(c, "kubectl delete ns wedakube-dev --wait=false")
	}()

	if err := EnsureNamespace(ctx, ssh, tgt.Namespace); err != nil {
		t.Fatalf("ensure ns: %v", err)
	}

	// 1) 首次 apply（nginx:alpine）
	if err := Apply(ctx, ssh, []byte(Render(tgt, "alpine"))); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if err := WaitRollout(ctx, ssh, tgt, emit); err != nil {
		t.Fatalf("first rollout: %v", err)
	}
	t.Log("=> first deploy Ready")

	// 2) set image 換 tag（nginx:1.27-alpine）-> rollout
	if err := SetImage(ctx, ssh, tgt, "1.27-alpine"); err != nil {
		t.Fatalf("set image: %v", err)
	}
	if err := WaitRollout(ctx, ssh, tgt, emit); err != nil {
		t.Fatalf("update rollout: %v", err)
	}
	t.Log("=> updated image Ready")

	// 3) rollback -> rollout
	if err := Rollback(ctx, ssh, tgt); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if err := WaitRollout(ctx, ssh, tgt, emit); err != nil {
		t.Fatalf("rollback rollout: %v", err)
	}
	t.Log("=> rollback Ready")
}
