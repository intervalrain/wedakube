
package cluster

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type SSH struct {
	alias string
	path  string
}

func NewSSH(alias string) *SSH {
	return &SSH{
		alias: alias,
		path: filepath.Join(os.TempDir(), "wedakube-cm-" + alias),
	}
}

func (s *SSH) opts() []string {
	return []string{
		"-o", "ControlMaster=auto",
		"-o", "ControlPath=" + s.path,
		"-o", "ControlPersist=120",
		"-o", "ConnectTimeout=8",
	}
}

func (s *SSH) Run(ctx context.Context, remoteCmd string) ([]byte, error) {
	args := append(s.opts(), s.alias, remoteCmd)
	cmd := exec.CommandContext(ctx, "ssh", args...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("ssh %q: %w: %s", remoteCmd, err, stderr.String())
	}
	return out, nil
}

// RunStdin 跟 Run 一樣，但把 stdin 餵給遠端指令（例如 kubectl apply -f -）。
func (s *SSH) RunStdin(ctx context.Context, remoteCmd string, stdin []byte) ([]byte, error) {
	args := append(s.opts(), s.alias, remoteCmd)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	cmd.Stdin = bytes.NewReader(stdin)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("ssh %q: %w: %s", remoteCmd, err, stderr.String())
	}
	return out, nil
}

func (s *SSH) Close() error {
	exec.Command("ssh", "-o", "ControlPath=" + s.path, "-O", "exit", s.alias).Run()
	return nil
}