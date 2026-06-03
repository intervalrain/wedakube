package cluster

import (
	"context"
	"testing"
	"time"

	"github.com/intervalrain/wedakube/internal/config"
)

// 整合測試：打真 cluster。go test -short 會跳過。
func TestDeploymentsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; needs cluster access")
	}

	// 用顯式身分（不靠 ~/.ssh/config alias），驗證 M3 的 ssh -i/-l 路徑。
	ssh := NewSSH(config.Host{
		Name:         "adlk-explicit",
		HostName:     "10.0.0.1",
		User:         "ubuntu",
		IdentityFile: "~/.ssh/private.key",
	})
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
		t.Logf("%-24s %-6s up=%d  age=%-5s %s", s.Name, s.Ready, s.UpToDate, s.Age, s.ShortImage())
	}
}

func TestSampleHelmParamsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration; needs cluster")
	}
	ssh := NewSSH(config.Host{
		Name: "adlk", HostName: "10.0.0.1", User: "ubuntu", IdentityFile: "~/.ssh/private.key",
	})
	defer ssh.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ns, err := ResolveWedaNamespace(ctx, ssh)
	if err != nil {
		t.Fatalf("ns: %v", err)
	}
	hp, err := NewKubectl(ssh, ns).SampleHelmParams(ctx)
	if err != nil {
		t.Fatalf("sample: %v", err)
	}
	t.Logf("tenantId=%s  path=%s  alias=%s  srp=%s", hp.TenantID, hp.TenantPath, hp.TenantAlias, hp.SrpName)
	t.Logf("domain=%s  eco=%s  apiKey.len=%d", hp.Domain, hp.EcoEndpoint, len(hp.EcoApiKey))
	t.Logf("registry=%s  project=%s", hp.Registry, hp.Project)

	for name, got := range map[string]string{
		"TenantID": hp.TenantID, "SrpName": hp.SrpName, "EcoApiKey": hp.EcoApiKey, "Registry": hp.Registry, "Project": hp.Project,
	} {
		if got == "" {
			t.Errorf("%s is empty", name)
		}
	}
	if hp.Registry != "registry.example.com" {
		t.Errorf("registry=%q, want registry.example.com", hp.Registry)
	}
	if hp.Project != "edge-coa" {
		t.Errorf("project=%q, want edge-coa", hp.Project)
	}
}
