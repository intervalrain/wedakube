package cluster

import (
	"context"
	"testing"
	"time"
)

// 整合測試：打真 cluster。go test -short 會跳過。
func TestDeploymentsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; needs cluster access")
	}

	ssh := NewSSH("my-cluster")
	defer ssh.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	ns, err := ResolveWedaNamespace(ctx, ssh)
	if err != nil {
		t.Fatalf("resolve ns: %v", err)
	}
	t.Logf("namespace = %s", ns)

	svcs, err := NewKubectl(ssh, ns).Deployments(ctx)
	if err != nil {
		t.Fatalf("deployments: %v", err)
	}
	if len(svcs) == 0 {
		t.Fatal("expected at least one deployment")
	}
	for _, s := range svcs {
		t.Logf("%-24s %-6s up=%d  %s", s.Name, s.Ready, s.UpToDate, s.ShortImage())
	}
}
