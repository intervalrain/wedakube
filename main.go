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

// seedDefaultHost：首次執行時，若沒有任何 host 就放入 dev cluster 預設值（L1 host form 做好前的便利）。
func seedDefaultHost(store *config.Store) {
	hosts, err := store.ListHosts()
	if err != nil || len(hosts) > 0 {
		return
	}
	store.PutHost(config.Host{
		Name:         "my-cluster",
		HostName:     "10.0.0.1",
		User:         "ubuntu",
		IdentityFile: "~/.ssh/private.key",
	})
}
