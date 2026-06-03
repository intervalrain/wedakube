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

// baseOpts 是 ssh / scp 都通用的旗標。
func (s *SSH) baseOpts() []string {
	o := []string{
		"-o", "ControlMaster=auto",
		"-o", "ControlPath=" + s.path,
		"-o", "ControlPersist=120",
		"-o", "ConnectTimeout=8",
	}
	if s.host.IdentityFile != "" {
		o = append(o, "-i", s.host.ExpandIdentity())
	}
	return o
}

// opts 是 ssh 專用（-l user）。
func (s *SSH) opts() []string {
	o := s.baseOpts()
	if s.host.User != "" {
		o = append(o, "-l", s.host.User)
	}
	return o
}

// scpDest 回傳 scp 用的目的地字串（user@host or host）。
func (s *SSH) scpDest() string {
	dest := s.host.Dest()
	if s.host.User != "" {
		return s.host.User + "@" + dest
	}
	return dest
}

// ScpUpload 把本機檔上傳到遠端路徑（複用 ControlMaster socket）。
func (s *SSH) ScpUpload(ctx context.Context, localPath, remotePath string) error {
	args := append(s.baseOpts(), localPath, s.scpDest()+":"+remotePath)
	cmd := exec.CommandContext(ctx, "scp", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("scp %s: %w: %s", localPath, err, stderr.String())
	}
	return nil
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
