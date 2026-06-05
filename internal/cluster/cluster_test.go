package cluster

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/intervalrain/wedakube/internal/config"
)

// testHostFromEnv 從 env 組裝整合測試用的 host。未設則 skip — 避免把任何
// 具體 cluster 資訊寫進原始碼。export:
//
//	KUBE_TEST_HOSTNAME, KUBE_TEST_USER (default: ubuntu), KUBE_TEST_KEY
func testHostFromEnv(t *testing.T) config.Host {
	t.Helper()
	host := os.Getenv("KUBE_TEST_HOSTNAME")
	if host == "" {
		t.Skip("integration test; set KUBE_TEST_HOSTNAME to enable")
	}
	user := os.Getenv("KUBE_TEST_USER")
	if user == "" {
		user = "ubuntu"
	}
	return config.Host{
		Name:         "kube-test",
		HostName:     host,
		User:         user,
		IdentityFile: os.Getenv("KUBE_TEST_KEY"),
	}
}

// 整合測試：打真 cluster。go test -short 會跳過。
func TestDeploymentsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; needs cluster access")
	}
	ssh := NewSSH(testHostFromEnv(t))
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
	ssh := NewSSH(testHostFromEnv(t))
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
	t.Logf("tenantId.len=%d  srp=%s  registry=%s  project=%s",
		len(hp.TenantID), hp.SrpName, hp.Registry, hp.Project)
	t.Logf("domain=%s  eco=%s  apiKey.len=%d", hp.Domain, hp.EcoEndpoint, len(hp.EcoApiKey))

	for name, got := range map[string]string{
		"TenantID": hp.TenantID, "SrpName": hp.SrpName, "EcoApiKey": hp.EcoApiKey,
		"Registry": hp.Registry, "Project": hp.Project,
	} {
		if got == "" {
			t.Errorf("%s is empty", name)
		}
	}
}
