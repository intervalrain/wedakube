package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSanitizeName(t *testing.T) {
	cases := map[string]string{
		"file-transfer": "file-transfer",
		"FileTransfer":  "filetransfer",
		"my_service":    "my-service",
		"weda IoT svc":  "weda-iot-svc",
		"_edge_":        "edge",
	}
	for in, want := range cases {
		if got := sanitizeName(in); got != want {
			t.Errorf("sanitizeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestVersionBase(t *testing.T) {
	cases := map[string]string{
		"v0.2.0_20260121.1": "v0.2.0",
		"v1.1.0":            "v1.1.0",
		"v0.1.0-dev":        "v0.1.0",
		"":                  "",
	}
	for in, want := range cases {
		if got := versionBase(in); got != want {
			t.Errorf("versionBase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseAppcfg(t *testing.T) {
	dir := t.TempDir()
	body := "name: file-transfer\nversion: v0.2.0_20260121.1\nport: 5001\n"
	if err := os.WriteFile(filepath.Join(dir, "appcfg.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	name, version := parseAppcfg(dir)
	if name != "file-transfer" {
		t.Errorf("name = %q, want file-transfer", name)
	}
	if version != "v0.2.0_20260121.1" {
		t.Errorf("version = %q, want v0.2.0_20260121.1", version)
	}
}

func TestDetectPort(t *testing.T) {
	dotnet := "FROM x\nENV ASPNETCORE_HTTP_PORTS=5001\nEXPOSE 5001\n"
	if got := detectPort(dotnet, 8080); got != 5001 {
		t.Errorf("dotnet port = %d, want 5001", got)
	}
	if got := detectPort("EXPOSE 8080", 80); got != 8080 {
		t.Errorf("expose port = %d, want 8080", got)
	}
	if got := detectPort("no ports declared", 1234); got != 1234 {
		t.Errorf("default port = %d, want 1234", got)
	}
}
