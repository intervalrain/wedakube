package deploy

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Source is a clean export of one commit of a repo, used as the docker build context.
//
// CI (the shared C# service pipeline) builds from a fresh checkout with submodules, so
// nothing gitignored or uncommitted ever reaches the image. Building from the working tree
// instead baked a developer's local appsettings.secrets.json into an image,
// and the service crashed on its localhost connection string. kube therefore builds what
// CI would build: the committed tree of a ref plus every submodule at its pinned commit.
type Source struct {
	Dir   string // export root; mirrors the repo's top level
	Root  string // the repo's top level the export was made from
	Rev   string // full commit id that was exported
	Dirty bool   // the working tree had uncommitted changes to tracked files (NOT exported)
}

// Cleanup removes the export.
func (s Source) Cleanup() {
	if s.Dir != "" {
		_ = os.RemoveAll(s.Dir)
	}
}

// ExportSource exports ref (a branch, tag or commit; "" means HEAD) of the repo containing
// dir into a new temporary directory. The working tree, the index and the checkout are not
// touched, so any branch can be built without switching to it.
func ExportSource(ctx context.Context, dir, ref string) (Source, error) {
	if ref == "" {
		ref = "HEAD"
	}
	root, err := gitOut(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return Source{}, fmt.Errorf("%s is not inside a git repository: %w", dir, err)
	}
	rev, err := gitOut(ctx, root, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if err != nil {
		return Source{}, fmt.Errorf("ref %q not found in %s (fetch it first?): %w", ref, root, err)
	}
	status, err := gitOut(ctx, root, "status", "--porcelain", "--untracked-files=no", "--ignore-submodules=dirty")
	if err != nil {
		return Source{}, fmt.Errorf("git status: %w", err)
	}

	out, err := os.MkdirTemp("", "kube-src-")
	if err != nil {
		return Source{}, err
	}
	src := Source{Dir: out, Root: root, Rev: rev, Dirty: status != ""}
	if err := exportTree(ctx, root, rev, out); err != nil {
		src.Cleanup()
		return Source{}, err
	}
	return src, nil
}

// exportTree writes the tree of rev in repo into dst, then recurses into its submodules.
func exportTree(ctx context.Context, repo, rev, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	archive := exec.CommandContext(ctx, "git", "-C", repo, "archive", "--format=tar", rev)
	extract := exec.CommandContext(ctx, "tar", "-x", "-f", "-", "-C", dst)
	pipe, err := archive.StdoutPipe()
	if err != nil {
		return err
	}
	extract.Stdin = pipe
	var aerr, xerr bytes.Buffer
	archive.Stderr, extract.Stderr = &aerr, &xerr
	if err := extract.Start(); err != nil {
		return err
	}
	if err := archive.Run(); err != nil {
		_ = extract.Wait()
		return fmt.Errorf("git archive %s in %s: %w: %s", short(rev), repo, err, strings.TrimSpace(aerr.String()))
	}
	if err := extract.Wait(); err != nil {
		return fmt.Errorf("extract %s: %w: %s", short(rev), err, strings.TrimSpace(xerr.String()))
	}

	tree, err := exec.CommandContext(ctx, "git", "-C", repo, "ls-tree", "-r", "-z", rev).Output()
	if err != nil {
		return fmt.Errorf("git ls-tree %s in %s: %w", short(rev), repo, err)
	}
	for _, l := range parseGitlinks(tree) {
		subRepo := filepath.Join(repo, l.Path)
		if _, err := gitOut(ctx, subRepo, "rev-parse", "--git-dir"); err != nil {
			return fmt.Errorf("submodule %s is not initialized; run `git submodule update --init` in %s", l.Path, repo)
		}
		if !hasCommit(ctx, subRepo, l.Rev) {
			// The pinned commit may be newer than the local submodule clone.
			_, _ = gitOut(ctx, subRepo, "fetch", "--quiet", "origin", l.Rev)
			if !hasCommit(ctx, subRepo, l.Rev) {
				return fmt.Errorf("submodule %s: pinned commit %s not found locally or on origin", l.Path, short(l.Rev))
			}
		}
		if err := exportTree(ctx, subRepo, l.Rev, filepath.Join(dst, l.Path)); err != nil {
			return err
		}
	}
	return nil
}

type gitlink struct {
	Path string
	Rev  string
}

// parseGitlinks picks the submodule entries (mode 160000) out of `git ls-tree -r -z`.
func parseGitlinks(out []byte) []gitlink {
	var links []gitlink
	for _, entry := range bytes.Split(out, []byte{0}) {
		meta, path, ok := strings.Cut(string(entry), "\t")
		if !ok {
			continue
		}
		f := strings.Fields(meta)
		if len(f) == 3 && f[0] == "160000" && f[1] == "commit" {
			links = append(links, gitlink{Path: path, Rev: f[2]})
		}
	}
	return links
}

func hasCommit(ctx context.Context, repo, rev string) bool {
	_, err := gitOut(ctx, repo, "cat-file", "-e", rev+"^{commit}")
	return err == nil
}

func gitOut(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("%w: %s", err, msg)
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func short(rev string) string {
	if len(rev) > 8 {
		return rev[:8]
	}
	return rev
}

// feedPATPlaceholder is what the repos commit in NuGet.Config in place of the feed PAT.
const feedPATPlaceholder = "PAT_TOKEN_HERE"

// InjectFeedPAT replaces the PAT placeholder in every NuGet.Config under dir, as CI does
// (`sed -i 's|PAT_TOKEN_HERE|$PAT|' <nugetConfigPath>`) before docker build. Repos whose
// Dockerfile reads ARG FEED_PAT keep working: the build args are still passed. Only call it
// on an export, never on a working tree. Returns the number of files changed.
func InjectFeedPAT(dir, pat string) (int, error) {
	if pat == "" {
		return 0, nil
	}
	n := 0
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(d.Name(), "nuget.config") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Contains(b, []byte(feedPATPlaceholder)) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		n++
		return os.WriteFile(path, bytes.ReplaceAll(b, []byte(feedPATPlaceholder), []byte(pat)), info.Mode().Perm())
	})
	return n, err
}

// rebase maps a path under from onto the same relative path under to.
func rebase(path, from, to string) (string, error) {
	rel, err := filepath.Rel(from, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s is outside the repository %s", path, from)
	}
	return filepath.Join(to, rel), nil
}
