package tui

import "testing"

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
