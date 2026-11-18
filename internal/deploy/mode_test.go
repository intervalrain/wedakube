package deploy

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/intervalrain/wedakube/internal/config"
)

func TestNeedsFull(t *testing.T) {
	cases := []struct {
		files []string
		full  bool
	}{
		// code only: `kubectl set image` is enough
		{[]string{"Svc/src/Svc.Application/Commands/DispatchClient.cs", "CHANGELOG.md"}, false},
		{nil, false},
		// appcfg drives env, traefik routes and ECO: only helmster + helm apply it
		{[]string{"appcfg.yaml"}, true},
		{[]string{"deploy/appcfg-srp.yaml"}, true},
		// a schema change needs the db-migrator pre-upgrade Job
		{[]string{"Svc/src/Svc.EntityFrameworkCore/Migrations/20260101_AddOrders.cs"}, true},
		{[]string{"Svc/src/Svc.DbMigrator/Program.cs"}, true},
		// a committed chart
		{[]string{"charts/my-service/values.yaml"}, true},
		// lookalikes stay image-only
		{[]string{"docs/migrations-guide.md", "src/MigrationsHelper.cs"}, false},
	}
	for _, c := range cases {
		full, why := needsFull(c.files)
		if full != c.full {
			t.Errorf("needsFull(%v) = %v (%s), want %v", c.files, full, why, c.full)
		}
		if full && why == "" {
			t.Errorf("needsFull(%v): a full deploy must say why", c.files)
		}
	}
}

func TestParseAppVersion(t *testing.T) {
	out := []byte(`[{"name":"my-service","chart":"my-service-0.0.0-develop-20260101.1","app_version":"develop-20260101.1"}]`)
	got, err := parseAppVersion(out, "my-service")
	if err != nil || got != "develop-20260101.1" {
		t.Errorf("parseAppVersion = %q, %v", got, err)
	}
	if _, err := parseAppVersion([]byte(`[]`), "my-service"); err == nil {
		t.Error("expected an error when the release is not listed")
	}
}

func TestParseMode(t *testing.T) {
	for in, want := range map[string]Mode{"": ModeAuto, "auto": ModeAuto, "image": ModeImage, "full": ModeFull} {
		if got, err := ParseMode(in); err != nil || got != want {
			t.Errorf("ParseMode(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := ParseMode("fast"); err == nil || !strings.Contains(err.Error(), "auto|image|full") {
		t.Errorf("ParseMode(fast) err = %v", err)
	}
}

func TestDecideMode(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	write(t, filepath.Join(repo, "appcfg.yaml"), "version: v0\n")
	write(t, filepath.Join(repo, "src", "App.cs"), "v1")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "base")
	base := git(t, repo, "rev-parse", "HEAD")

	store := config.NewStore(filepath.Join(t.TempDir(), "state.json"))
	tgt := config.Target{ImageRepo: "reg/proj/svc"}
	if err := store.RecordBuild(tgt.ImageRepo, "v1_1", base); err != nil {
		t.Fatal(err)
	}
	at := func(ref string) Source {
		src, err := ExportSource(ctx, repo, ref)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(src.Cleanup)
		return src
	}

	// code-only change since the release's full deploy → image
	write(t, filepath.Join(repo, "src", "App.cs"), "v2")
	git(t, repo, "commit", "-q", "-am", "code")
	if m, why := decideMode(ctx, store, tgt, at("HEAD"), "v1_1", nil); m != ModeImage {
		t.Errorf("code only: %s (%s), want image", m, why)
	}

	// appcfg changed → full
	write(t, filepath.Join(repo, "appcfg.yaml"), "version: v0\nenv: x\n")
	git(t, repo, "commit", "-q", "-am", "appcfg")
	if m, why := decideMode(ctx, store, tgt, at("HEAD"), "v1_1", nil); m != ModeFull || !strings.Contains(why, "appcfg") {
		t.Errorf("appcfg: %s (%s), want full because of appcfg", m, why)
	}

	// release deployed by CI (tag kube never built) → full, baseline unknown
	if m, why := decideMode(ctx, store, tgt, at("HEAD"), "develop-20260101.1", nil); m != ModeFull || !strings.Contains(why, "not built by kube") {
		t.Errorf("unknown baseline: %s (%s), want full", m, why)
	}

	// app_version unreadable → full
	if m, _ := decideMode(ctx, store, tgt, at("HEAD"), "", errors.New("ssh down")); m != ModeFull {
		t.Errorf("unreadable app_version: %s, want full", m)
	}
}
