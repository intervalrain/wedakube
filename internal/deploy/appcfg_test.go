package deploy

import (
	"strings"
	"testing"
)

// Reproduces a helmster deploy: helmster bakes jobs.<name>.imageTag from appcfg's
// top-level version, so the db-migrator Job pulled v0.1.0_20260101.1 (not in Harbor).
// CI runs `yq -i ".version = IMAGE_TAG"` before `helmster gen`; kube must do the same.
func TestWithAppcfgVersion_ReplacesTopLevelVersionOnly(t *testing.T) {
	in := "name: my-service\nversion: v0.1.0_20260101.1\nbackingServices:\n  - name: postgresql\n    version: \"16\"\n"
	got := string(withAppcfgVersion([]byte(in), "v0.2.0_20260102.1"))

	if !strings.Contains(got, "\nversion: v0.2.0_20260102.1\n") {
		t.Errorf("top-level version not replaced:\n%s", got)
	}
	if !strings.Contains(got, "    version: \"16\"\n") {
		t.Errorf("a nested version must stay untouched:\n%s", got)
	}
	if strings.Contains(got, "v0.1.0_20260101.1") {
		t.Errorf("old version still present:\n%s", got)
	}
}

func TestWithAppcfgVersion_QuotedAndCommented(t *testing.T) {
	in := "version: \"v0.1.0\" # bumped by CI\nname: x\n"
	got := string(withAppcfgVersion([]byte(in), "v9"))
	if !strings.HasPrefix(got, "version: v9\n") {
		t.Errorf("got:\n%s", got)
	}
}

func TestWithAppcfgVersion_AddsVersionWhenMissing(t *testing.T) {
	got := string(withAppcfgVersion([]byte("name: x\n"), "v9"))
	if !strings.HasPrefix(got, "version: v9\n") || !strings.Contains(got, "name: x\n") {
		t.Errorf("got:\n%s", got)
	}
}

// A failed helm upgrade used to print the whole command including global.eco.apiKey.
func TestRedact(t *testing.T) {
	got := redact("--set global.eco.apiKey='AK1-secret' failed: AK1-secret", "AK1-secret", "")
	if strings.Contains(got, "AK1-secret") {
		t.Errorf("secret leaked: %s", got)
	}
	if !strings.Contains(got, "<redacted>") {
		t.Errorf("expected a <redacted> marker: %s", got)
	}
}
