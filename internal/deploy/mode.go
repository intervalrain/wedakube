package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/intervalrain/wedakube/internal/cluster"
)

// Mode is how a helm-managed service is rolled out.
//
//   - ModeImage: clean build + push, then `kubectl set image` on the existing Deployment.
//     Fast, and enough when only code changed. Skips helm, so no db-migrator Job, no
//     appcfg (env, traefik routes, ECO) changes, and helm history keeps the old tag.
//   - ModeFull: clean build + push, then helmster gen + helm upgrade --install, as CI does:
//     the pre-upgrade db-migrator Job runs and appcfg changes are applied.
//   - ModeAuto: ModeFull when the commits since the release's last full deploy touch
//     appcfg, migrations, the DbMigrator or a chart, or when that baseline is unknown;
//     ModeImage otherwise.
type Mode string

const (
	ModeAuto  Mode = "auto"
	ModeImage Mode = "image"
	ModeFull  Mode = "full"
)

// ParseMode reads a --mode value; "" means auto.
func ParseMode(s string) (Mode, error) {
	switch Mode(s) {
	case "", ModeAuto:
		return ModeAuto, nil
	case ModeImage, ModeFull:
		return Mode(s), nil
	}
	return "", fmt.Errorf("unknown mode %q (want auto|image|full)", s)
}

// needsFull reports whether any changed path needs helm (helmster + db-migrator), and why.
func needsFull(files []string) (bool, string) {
	var why []string
	for _, f := range files {
		dirs := strings.Split(path.Dir(f), "/")
		base := path.Base(f)
		switch {
		case strings.HasPrefix(base, "appcfg") && (strings.HasSuffix(base, ".yaml") || strings.HasSuffix(base, ".yml")):
			why = append(why, f+" (appcfg)")
		case hasDir(dirs, func(d string) bool { return d == "Migrations" }):
			why = append(why, f+" (migration)")
		case hasDir(dirs, func(d string) bool { return strings.HasSuffix(d, "DbMigrator") }):
			why = append(why, f+" (DbMigrator)")
		case hasDir(dirs, func(d string) bool { return d == "charts" }):
			why = append(why, f+" (chart)")
		}
	}
	if len(why) == 0 {
		return false, ""
	}
	if len(why) > 3 {
		why = append(why[:3], fmt.Sprintf("and %d more", len(why)-3))
	}
	return true, strings.Join(why, ", ")
}

func hasDir(dirs []string, match func(string) bool) bool {
	for _, d := range dirs {
		if match(d) {
			return true
		}
	}
	return false
}

// parseAppVersion reads a release's app_version from `helm list -o json`.
func parseAppVersion(out []byte, release string) (string, error) {
	var rels []struct {
		Name       string `json:"name"`
		AppVersion string `json:"app_version"`
	}
	if err := json.Unmarshal(out, &rels); err != nil {
		return "", fmt.Errorf("parse helm list: %w", err)
	}
	for _, r := range rels {
		if r.Name == release {
			return r.AppVersion, nil
		}
	}
	return "", fmt.Errorf("release %s not found", release)
}

func helmAppVersion(ctx context.Context, ssh *cluster.SSH, release, ns string) (string, error) {
	out, err := ssh.Run(ctx, fmt.Sprintf("helm list -n %s -o json --filter '^%s$'", ns, release))
	if err != nil {
		return "", err
	}
	return parseAppVersion(out, release)
}

// changedFiles lists the paths changed between two commits of the repo at root.
func changedFiles(ctx context.Context, root, from, to string) ([]string, error) {
	out, err := gitOut(ctx, root, "diff", "--name-only", from, to)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}
