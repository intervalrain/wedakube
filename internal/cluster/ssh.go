package cluster

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/intervalrain/wedakube/internal/config"
)

// SSH 透過 OpenSSH ControlMaster 複用一條連線，在遠端 node 上跑指令。
type SSH struct {
	host config.Host
	path string
}

func NewSSH(h config.Host) *SSH {
	key := h.Name
	if key == "" {
		key = h.Dest()
	}
	return &SSH{
		host: h,
		path: filepath.Join(os.TempDir(), "wedakube-cm-"+sanitize(key)),
	}
}

func sanitize(s string) string {
	return strings.NewReplacer("/", "_", " ", "_", ":", "_", "@", "_").Replace(s)
}

func (s *SSH) opts() []string {
	o := []string{
		"-o", "ControlMaster=auto",
		"-o", "ControlPath=" + s.path,
		"-o", "ControlPersist=120",
		"-o", "ConnectTimeout=8",
	}
	if s.host.IdentityFile != "" {
		o = append(o, "-i", s.host.ExpandIdentity())
	}
	if s.host.User != "" {
		o = append(o, "-l", s.host.User)
	}
	return o
}

func (s *SSH) Run(ctx context.Context, remoteCmd string) ([]byte, error) {
	args := append(s.opts(), s.host.Dest(), remoteCmd)
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
	args := append(s.opts(), s.host.Dest(), remoteCmd)
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
	exec.Command("ssh", "-o", "ControlPath="+s.path, "-O", "exit", s.host.Dest()).Run()
	return nil
}
