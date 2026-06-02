package deploy

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"

	"github.com/intervalrain/wedakube/internal/config"
)

// BuildAndPush 用 buildx 建 linux/amd64 image 並推到 registry，一步完成。
// 本地是 arm64、node 是 amd64，所以 --platform 不可省。FEED_PAT 由環境變數帶入私有 NuGet restore。
func BuildAndPush(ctx context.Context, t config.Target, tag string, emit Emitter) error {
	image := t.ImageRepo + ":" + tag

	args := []string{
		"buildx", "build",
		"--platform", "linux/amd64",
		"-f", t.Dockerfile,
		"-t", image,
		"--push",
	}
	if pat := os.Getenv("FEED_PAT"); pat != "" {
		args = append(args, "--build-arg", "FEED_PAT="+pat)
	}
	args = append(args, t.Context)

	cmd := exec.CommandContext(ctx, "docker", args...)

	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw // buildx 把進度寫到 stderr，合流一起串

	if err := cmd.Start(); err != nil {
		return err
	}

	done := make(chan struct{})
	go func() {
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			emit(Event{Phase: "build", Msg: sc.Text(), Pct: -1})
		}
		close(done)
	}()

	err := cmd.Wait()
	pw.Close()
	<-done
	return err
}
