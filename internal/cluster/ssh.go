package cluster

import (
	"bytes"
	"context"
	"fmt"
	"io"
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

// ExecCmd 組一條會接管使用者 TTY 的 ssh 子程序（給 kubectl exec -it 之類的互動指令）。
// 呼叫端通常透過 bubbletea 的 tea.ExecProcess 來跑：那會暫停 TUI、把終端機交給這個程序，
// 程序結束（user 打 exit / Ctrl-D）後再把 TUI 喚醒。-t 是為了讓 ssh 在遠端配 PTY。
func (s *SSH) ExecCmd(ctx context.Context, remoteCmd string) *exec.Cmd {
	args := append(s.opts(), "-t", s.host.Dest(), remoteCmd)
	return exec.CommandContext(ctx, "ssh", args...)
}

// Stream 跑一條會持續輸出的遠端指令（例如 kubectl logs -f）。
// 回傳的 reader 串接 stdout+stderr；呼叫端 ctx cancel 就會把 ssh 子程序殺掉。
func (s *SSH) Stream(ctx context.Context, remoteCmd string) (io.Reader, error) {
	args := append(s.opts(), s.host.Dest(), remoteCmd)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw
	if err := cmd.Start(); err != nil {
		pw.Close()
		return nil, err
	}
	go func() {
		_ = cmd.Wait()
		pw.Close()
	}()
	return pr, nil
}

// PortForward 開一條 ssh -L 通道，同時在遠端跑 kubectl port-forward。
// 兩件事綁同一個 ssh process：ctx cancel → 子程序死 → 遠端 kubectl 也跟著斷。
func (s *SSH) PortForward(ctx context.Context, localPort, remotePort int, remoteCmd string) error {
	args := append(s.opts(),
		fmt.Sprintf("-L%d:localhost:%d", localPort, remotePort),
		s.host.Dest(),
		remoteCmd,
	)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func (s *SSH) Close() error {
	exec.Command("ssh", "-o", "ControlPath="+s.path, "-O", "exit", s.host.Dest()).Run()
	return nil
}
