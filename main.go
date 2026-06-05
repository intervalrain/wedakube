package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/intervalrain/wedakube/internal/config"
	"github.com/intervalrain/wedakube/internal/tui"
)

// 編譯時透過 ldflags -X main.version=… 注入；參考 Makefile。
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "deploy":
			runDeploy(os.Args[2:])
			return
		case "version", "--version", "-v":
			fmt.Printf("kube %s (%s, built %s)\n", version, commit, date)
			return
		}
	}
	runTUI()
}

func runTUI() {
	store, err := config.DefaultStore()
	if err != nil {
		fmt.Fprintln(os.Stderr, "store:", err)
		os.Exit(1)
	}
	seedDefaultHost(store)

	p := tea.NewProgram(tui.NewApp(tui.NewHosts(store)), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// seedDefaultHost：首次執行時，若沒有任何 host 且環境變數有設，
// 就放一個預設 host 進來（L1 host form 做好前的便利）。
// 需要設：KUBE_DEFAULT_HOST（顯示名）, KUBE_DEFAULT_HOSTNAME（IP/主機名）,
// KUBE_DEFAULT_USER（ssh 使用者）, KUBE_DEFAULT_KEY（私鑰路徑）。
func seedDefaultHost(store *config.Store) {
	hosts, err := store.ListHosts()
	if err != nil || len(hosts) > 0 {
		return
	}
	name := os.Getenv("KUBE_DEFAULT_HOST")
	hostname := os.Getenv("KUBE_DEFAULT_HOSTNAME")
	if name == "" || hostname == "" {
		return // 沒設環境變數就不 seed，使用者自己編 state.json 或用未來的 Host Form
	}
	store.PutHost(config.Host{
		Name:         name,
		HostName:     hostname,
		User:         os.Getenv("KUBE_DEFAULT_USER"),
		IdentityFile: os.Getenv("KUBE_DEFAULT_KEY"),
	})
}
