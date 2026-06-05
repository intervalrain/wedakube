package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/intervalrain/wedakube/internal/cluster"
	"github.com/intervalrain/wedakube/internal/config"
	"github.com/intervalrain/wedakube/internal/deploy"
)

// runDeploy 是 M2 的 headless 進入點：go run . deploy <repoPath>
// 真正接進 TUe 之前，先用這個跑完整 pipeline。
// 前置：(1) FEED_PAT 已設定（env 或 secrets.json）  (2) docker login <your-registry>
func runDeploy(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: wedakube deploy <repoPath>")
		os.Exit(2)
	}
	repo := args[0]

	store, err := config.DefaultStore()
	if err != nil {
		fmt.Fprintln(os.Stderr, "store:", err)
		os.Exit(1)
	}
	t, ok, err := store.GetTarget(repo)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load target:", err)
		os.Exit(1)
	}
	if !ok {
		fmt.Fprintf(os.Stderr, "no deploy target configured for %s\n", repo)
		os.Exit(1)
	}

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
	tag, err := deploy.Deploy(context.Background(), ssh, store, t, hp, date, feedPAT, true, emit)
	if err != nil {
		fmt.Fprintln(os.Stderr, "deploy failed:", err)
		os.Exit(1)
	}
	fmt.Println("✓ deployed", t.Service, "=>", tag)
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
