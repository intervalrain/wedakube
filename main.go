package main

import (
	"context"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/intervalrain/wedakube/internal/cluster"
	"github.com/intervalrain/wedakube/internal/tui"
)

const sshAlias = "my-cluster"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "deploy" {
		runDeploy(os.Args[2:])
		return
	}
	runTUI()
}

func runTUI() {
	ssh := cluster.NewSSH(sshAlias)
	defer ssh.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	ns, err := cluster.ResolveWedaNamespace(ctx, ssh)
	cancel()
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot resolve -weda namespace:", err)
		os.Exit(1)
	}

	kc := cluster.NewKubectl(ssh, ns)

	p := tea.NewProgram(tui.New(kc), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
