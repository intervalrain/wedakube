package deploy

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"

	"github.com/intervalrain/wedakube/internal/config"
)

// ResolveFeedPAT 取得私有 NuGet restore 用的 PAT：環境變數優先，否則用存檔的 secret。
func ResolveFeedPAT(store *config.Store) string {
	if v := os.Getenv("FEED_PAT"); v != "" {
		return v
	}
	if store != nil {
		if v, err := store.GetSecret("FEED_PAT"); err == nil {
			return v
		}
	}
	return ""
}

// DefaultPlatform 是偵測不到 node 架構時的保底值（dev 叢集絕大多數是 amd64）。
const DefaultPlatform = "linux/amd64"

// BuildAndPush 用 buildx 建指定 platform 的 image 並推到 registry，一步完成。
// 本地架構（如 arm64 Mac）常和 node 不同，所以 --platform 不可省；
// platform 由 Deploy 在 build 前對目標 node 偵測而來。feedPAT 帶入私有 NuGet restore。
func BuildAndPush(ctx context.Context, t config.Target, tag, platform, feedPAT string, emit Emitter) error {
	image := t.ImageRepo + ":" + tag
	if platform == "" {
		platform = DefaultPlatform
	}

	args := []string{
		"buildx", "build",
		"--platform", platform,
		"-f", t.Dockerfile,
		"-t", image,
		"--push",
	}
	if feedPAT != "" {
		args = append(args, "--build-arg", "FEED_PAT="+feedPAT)
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
