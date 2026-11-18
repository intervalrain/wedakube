package deploy

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/intervalrain/wedakube/internal/config"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "protocol.file.allow=always",
		"-c", "user.name=test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	return dir
}

// Reproduces a crash: a gitignored local appsettings.secrets.json and an
// uncommitted edit were baked into the image because the build context was the working tree.
// CI builds a clean checkout; so must kube.
func TestExportSource_LeavesOutIgnoredAndUncommittedFiles(t *testing.T) {
	repo := newRepo(t)
	write(t, filepath.Join(repo, ".gitignore"), "**/appsettings.secrets.json\n")
	write(t, filepath.Join(repo, "app", "appsettings.json"), `{"db":"committed"}`)
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "init")

	write(t, filepath.Join(repo, "app", "appsettings.secrets.json"), `{"db":"Host=localhost"}`)
	write(t, filepath.Join(repo, "app", "appsettings.json"), `{"db":"uncommitted"}`)

	src, err := ExportSource(context.Background(), repo, "HEAD")
	if err != nil {
		t.Fatalf("ExportSource: %v", err)
	}
	defer src.Cleanup()

	if _, err := os.Stat(filepath.Join(src.Dir, "app", "appsettings.secrets.json")); !os.IsNotExist(err) {
		t.Errorf("gitignored secrets file must not be exported (stat err = %v)", err)
	}
	got, _ := os.ReadFile(filepath.Join(src.Dir, "app", "appsettings.json"))
	if string(got) != `{"db":"committed"}` {
		t.Errorf("appsettings.json = %s, want the committed content", got)
	}
	if !src.Dirty {
		t.Error("Dirty = false, want true for a working tree with uncommitted edits")
	}
	if src.Rev != git(t, repo, "rev-parse", "HEAD") {
		t.Errorf("Rev = %s, want HEAD", src.Rev)
	}
}

func TestExportSource_BuildsAnotherRefWithoutCheckingItOut(t *testing.T) {
	repo := newRepo(t)
	write(t, filepath.Join(repo, "v.txt"), "main")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "main")
	git(t, repo, "checkout", "-q", "-b", "feature")
	write(t, filepath.Join(repo, "v.txt"), "feature")
	git(t, repo, "commit", "-q", "-am", "feature")
	git(t, repo, "checkout", "-q", "main")

	src, err := ExportSource(context.Background(), repo, "feature")
	if err != nil {
		t.Fatalf("ExportSource: %v", err)
	}
	defer src.Cleanup()
	if got, _ := os.ReadFile(filepath.Join(src.Dir, "v.txt")); string(got) != "feature" {
		t.Errorf("v.txt = %q, want the feature branch content", got)
	}
	if b := git(t, repo, "branch", "--show-current"); b != "main" {
		t.Errorf("checkout moved to %q; exporting a ref must not touch the working tree", b)
	}
}

// A worktree that failed to build had a stale copy in its submodule path; the export
// must contain the submodule at the commit the superproject pins, not whatever is on disk.
func TestExportSource_IncludesSubmoduleAtPinnedCommit(t *testing.T) {
	sub := newRepo(t)
	write(t, filepath.Join(sub, "Errors.cs"), "v1")
	git(t, sub, "add", ".")
	git(t, sub, "commit", "-q", "-m", "v1")

	repo := newRepo(t)
	write(t, filepath.Join(repo, "README.md"), "x")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "init")
	git(t, repo, "submodule", "add", "-q", sub, "src/ErrorCode")
	git(t, repo, "commit", "-q", "-m", "add submodule")

	// The checked-out submodule moves ahead, but the superproject still pins v1.
	subWT := filepath.Join(repo, "src", "ErrorCode")
	write(t, filepath.Join(subWT, "Errors.cs"), "v2")
	git(t, subWT, "commit", "-q", "-am", "v2")

	src, err := ExportSource(context.Background(), repo, "HEAD")
	if err != nil {
		t.Fatalf("ExportSource: %v", err)
	}
	defer src.Cleanup()
	if got, _ := os.ReadFile(filepath.Join(src.Dir, "src", "ErrorCode", "Errors.cs")); string(got) != "v1" {
		t.Errorf("submodule Errors.cs = %q, want the pinned v1", got)
	}
}

func TestExportSource_CleanupRemovesTheExport(t *testing.T) {
	repo := newRepo(t)
	write(t, filepath.Join(repo, "a"), "a")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "a")
	src, err := ExportSource(context.Background(), repo, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	src.Cleanup()
	if _, err := os.Stat(src.Dir); !os.IsNotExist(err) {
		t.Errorf("export dir still exists after Cleanup (err = %v)", err)
	}
}

func TestParseGitlinks(t *testing.T) {
	out := "100644 blob aaaa\tREADME.md\x00160000 commit 1111aaaa\tSvc/src/ErrorCode\x00" +
		"160000 commit 2222bbbb\tSvc/src/Contracts\x00"
	got := parseGitlinks([]byte(out))
	want := []gitlink{
		{Path: "Svc/src/ErrorCode", Rev: "1111aaaa"},
		{Path: "Svc/src/Contracts", Rev: "2222bbbb"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

// A Dockerfile without ARG FEED_PAT: like CI, the placeholder in NuGet.Config is replaced
// before docker build (in the export, never in the user's repo).
func TestInjectFeedPAT(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "Svc", "NuGet.Config"), `<add key="ClearTextPassword" value="PAT_TOKEN_HERE" />`)
	write(t, filepath.Join(dir, "Other", "nuget.config"), `value="PAT_TOKEN_HERE" value="PAT_TOKEN_HERE"`)
	write(t, filepath.Join(dir, "plain", "NuGet.Config"), `no placeholder`)
	write(t, filepath.Join(dir, "README.md"), `PAT_TOKEN_HERE`)

	n, err := InjectFeedPAT(dir, "s3cret")
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("files changed = %d, want 2", n)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "Svc", "NuGet.Config")); !strings.Contains(string(got), `value="s3cret"`) {
		t.Errorf("NuGet.Config not injected: %s", got)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "Other", "nuget.config")); strings.Contains(string(got), "PAT_TOKEN_HERE") {
		t.Errorf("every placeholder must be replaced: %s", got)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "README.md")); string(got) != "PAT_TOKEN_HERE" {
		t.Errorf("only NuGet config files are touched, README.md = %s", got)
	}
	if n, _ := InjectFeedPAT(dir, ""); n != 0 {
		t.Errorf("an empty PAT must change nothing, changed %d", n)
	}
}

func TestOnExport_PointsDockerfileAndContextAtTheExport(t *testing.T) {
	repo := newRepo(t)
	write(t, filepath.Join(repo, "Dockerfile"), "FROM scratch")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "init")
	src, err := ExportSource(context.Background(), repo, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	defer src.Cleanup()

	got, err := onExport(config.Target{Context: repo, Dockerfile: filepath.Join(repo, "Dockerfile")}, src)
	if err != nil {
		t.Fatalf("onExport: %v", err)
	}
	if got.Context != src.Dir || got.Dockerfile != filepath.Join(src.Dir, "Dockerfile") {
		t.Errorf("onExport = %+v, want context %s", got, src.Dir)
	}
	if _, err := onExport(config.Target{Context: t.TempDir()}, src); err == nil {
		t.Error("a build context outside the repository must be refused")
	}
}

func TestRebase(t *testing.T) {
	got, err := rebase("/repo/sub/Dockerfile", "/repo", "/tmp/x")
	if err != nil || got != "/tmp/x/sub/Dockerfile" {
		t.Errorf("rebase = %q, %v", got, err)
	}
	got, err = rebase("/repo", "/repo", "/tmp/x")
	if err != nil || got != "/tmp/x" {
		t.Errorf("rebase root = %q, %v", got, err)
	}
	if _, err := rebase("/elsewhere/Dockerfile", "/repo", "/tmp/x"); err == nil {
		t.Error("a path outside the repo must be refused")
	}
}
