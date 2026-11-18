package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/intervalrain/wedakube/internal/cluster"
	"github.com/intervalrain/wedakube/internal/config"
	"github.com/intervalrain/wedakube/internal/deploy"
)

// runDeploy 是 CLI 模式的部署進入點：kube deploy [repoPath]
// repoPath 省略或 "." = 當下目錄；target 需先在 TUI wizard 設定過。
// 有用 `kube ctx select` 選 server 時，優先部署到該 server。
// 前置：(1) FEED_PAT 已設定（env 或 secrets.json）  (2) docker login <your-registry>
func runDeploy(args []string) {
	opts, err := parseDeployArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprint(os.Stderr, deployUsage)
		os.Exit(2)
	}
	if opts.help {
		fmt.Print(deployUsage)
		return
	}
	repo := opts.repo

	store, err := config.DefaultStore()
	if err != nil {
		fmt.Fprintln(os.Stderr, "store:", err)
		os.Exit(1)
	}
	t, ok := lookupTarget(store, repo)
	if !ok {
		fmt.Fprintf(os.Stderr, "no deploy target configured for %s\nrun `kube` and use the wizard (w) to configure this repo first\n", repo)
		os.Exit(1)
	}

	// ctx 覆寫：CLI 選了 server 就部到那台；換台時 namespace 改用該台的推導值。
	if cur, _ := store.CurrentHost(); cur != "" && cur != t.Host {
		h, ok, _ := store.GetHost(cur)
		if !ok {
			fmt.Fprintf(os.Stderr, "current server %q no longer exists — fix with `kube ctx select`\n", cur)
			os.Exit(1)
		}
		t.Host = h.Name
		t.SSHAlias = h.Name
		if ns := h.DerivedNamespace(); ns != "" {
			t.Namespace = ns
		}
	}
	fmt.Printf("deploying %s => %s (ns %s)\n", t.Service, t.Host, t.Namespace)

	ssh := cluster.NewSSH(resolveHost(store, t))
	defer ssh.Close()

	emit := func(e deploy.Event) {
		if e.Phase == "rollout" {
			fmt.Printf("[rollout] %s (%.0f%%)\n", e.Msg, e.Pct*100)
		} else {
			fmt.Printf("[%s] %s\n", e.Phase, e.Msg)
		}
	}

	date := time.Now().Format("20060102")
	feedPAT := deploy.ResolveFeedPAT(store)
	var hp config.HelmParams
	if h, ok, _ := store.GetHost(t.Host); ok {
		hp = h.Helm
	} else if h, ok, _ := store.GetHost(t.SSHAlias); ok {
		hp = h.Helm
	}
	tag, err := deploy.Deploy(context.Background(), ssh, store, t, hp, date, feedPAT, opts.ref, true, emit)
	if err != nil {
		fmt.Fprintln(os.Stderr, "deploy failed:", err)
		os.Exit(1)
	}
	fmt.Println("✓ deployed", t.Service, "=>", tag)
}

// lookupTarget 用 repoPath 查 target：先照原樣，再試絕對路徑，最後試解完 symlink 的路徑
// （wizard 存的是絕對路徑，CLI 常給 "." 或相對路徑）。
func lookupTarget(store *config.Store, repo string) (config.Target, bool) {
	tryKeys := []string{repo}
	if abs, err := filepath.Abs(repo); err == nil {
		tryKeys = append(tryKeys, abs)
		if real, err := filepath.EvalSymlinks(abs); err == nil {
			tryKeys = append(tryKeys, real)
		}
	}
	for _, k := range tryKeys {
		if t, ok, err := store.GetTarget(k); err == nil && ok {
			return t, true
		}
	}
	return config.Target{}, false
}

// resolveHost 從 target 取得連線設定：優先用 t.Host 查 store，否則退回 SSHAlias（alias-only）。
func resolveHost(store *config.Store, t config.Target) config.Host {
	if t.Host != "" {
		if h, ok, _ := store.GetHost(t.Host); ok {
			return h
		}
	}
	return config.Host{Name: t.SSHAlias, Alias: t.SSHAlias, Namespace: t.Namespace}
}

const deployUsage = `usage: kube deploy [--ref <branch|tag|commit>] [repoPath]

Builds a clean export of the ref (default HEAD) with its submodules at their pinned
commits, like CI: uncommitted and gitignored files never reach the image. The checkout
is not touched, so any branch can be deployed without switching to it.
`

type deployOpts struct {
	repo string
	ref  string
	help bool
}

// parseDeployArgs reads `[--ref R | --ref=R] [-h|--help] [repoPath]`.
func parseDeployArgs(args []string) (deployOpts, error) {
	o := deployOpts{repo: "."}
	repoSet := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			o.help = true
		case a == "--ref":
			if i+1 >= len(args) || args[i+1] == "" {
				return o, fmt.Errorf("--ref needs a value")
			}
			i++
			o.ref = args[i]
		case strings.HasPrefix(a, "--ref="):
			o.ref = strings.TrimPrefix(a, "--ref=")
			if o.ref == "" {
				return o, fmt.Errorf("--ref needs a value")
			}
		case strings.HasPrefix(a, "-"):
			return o, fmt.Errorf("unknown flag %s", a)
		default:
			if repoSet {
				return o, fmt.Errorf("only one repoPath is allowed, got %q and %q", o.repo, a)
			}
			o.repo, repoSet = a, true
		}
	}
	return o, nil
}
