package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/intervalrain/wedakube/internal/config"
)

// runCtx 處理 `kube ctx …`：管理 CLI 模式下的目前 server（host）。
//
//	kube ctx            顯示目前選擇
//	kube ctx ls         列出所有 host，* 標記目前選擇
//	kube ctx select <s> 選擇 host（支援唯一前綴）
func runCtx(args []string) {
	store, err := config.DefaultStore()
	if err != nil {
		fmt.Fprintln(os.Stderr, "store:", err)
		os.Exit(1)
	}

	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "", "current":
		cur, err := store.CurrentHost()
		if err != nil {
			fmt.Fprintln(os.Stderr, "ctx:", err)
			os.Exit(1)
		}
		if cur == "" {
			fmt.Println("(no server selected — use `kube ctx select <server>`)")
			return
		}
		fmt.Println(cur)
	case "ls", "list":
		ctxList(store)
	case "select", "use":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: kube ctx select <server>")
			os.Exit(2)
		}
		ctxSelect(store, args[1])
	default:
		fmt.Fprintf(os.Stderr, "unknown ctx command %q\n\nusage:\n  kube ctx            show current server\n  kube ctx ls         list servers\n  kube ctx select <s> select server\n", sub)
		os.Exit(2)
	}
}

func ctxList(store *config.Store) {
	hosts, err := store.ListHosts()
	if err != nil {
		fmt.Fprintln(os.Stderr, "ctx:", err)
		os.Exit(1)
	}
	if len(hosts) == 0 {
		fmt.Println("(no hosts — add one in the TUI with `kube`, key n)")
		return
	}
	cur, _ := store.CurrentHost()

	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	fmt.Fprintln(w, "  NAME\tDEST\tNAMESPACE")
	for _, h := range hosts {
		mark := " "
		if h.Name == cur {
			mark = "*"
		}
		dest := h.Dest()
		if h.User != "" {
			dest = h.User + "@" + dest
		}
		fmt.Fprintf(w, "%s %s\t%s\t%s\n", mark, h.Name, dest, h.DerivedNamespace())
	}
	w.Flush()
}

func ctxSelect(store *config.Store, name string) {
	hosts, err := store.ListHosts()
	if err != nil {
		fmt.Fprintln(os.Stderr, "ctx:", err)
		os.Exit(1)
	}
	match, candidates := resolveHostName(hosts, name)
	if match == "" {
		if len(candidates) > 1 {
			fmt.Fprintf(os.Stderr, "%q is ambiguous — matches:\n", name)
			for _, n := range candidates {
				fmt.Fprintln(os.Stderr, "  "+n)
			}
		} else {
			fmt.Fprintf(os.Stderr, "no host matches %q — available:\n", name)
			for _, h := range hosts {
				fmt.Fprintln(os.Stderr, "  "+h.Name)
			}
		}
		os.Exit(1)
	}
	if err := store.SetCurrentHost(match); err != nil {
		fmt.Fprintln(os.Stderr, "ctx:", err)
		os.Exit(1)
	}
	fmt.Println("✓ current server:", match)
}

// resolveHostName 先找完全相符，再退回唯一前綴。
// 模糊時 match 回空字串，candidates 帶所有前綴相符者供錯誤訊息用。
func resolveHostName(hosts []config.Host, name string) (match string, candidates []string) {
	for _, h := range hosts {
		if h.Name == name {
			return h.Name, nil
		}
		if strings.HasPrefix(h.Name, name) {
			candidates = append(candidates, h.Name)
		}
	}
	if len(candidates) == 1 {
		return candidates[0], candidates
	}
	return "", candidates
}
