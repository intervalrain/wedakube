package eco

import "testing"

func TestSplitStatus(t *testing.T) {
	cases := []struct {
		name string
		in   string
		code int
		body string
		err  bool
	}{
		{"body+status", `{"data":[]}` + "\n200", 200, `{"data":[]}`, false},
		{"empty body", "\n204", 204, "", false},
		{"multiline body", "line1\nline2\n404", 404, "line1\nline2", false},
		{"status only", "200", 200, "", false},
		{"garbage", "not a number", 0, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, body, err := splitStatus([]byte(c.in))
			if c.err {
				if err == nil {
					t.Fatalf("expected error, got code=%d", code)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if code != c.code {
				t.Errorf("code = %d, want %d", code, c.code)
			}
			if string(body) != c.body {
				t.Errorf("body = %q, want %q", body, c.body)
			}
		})
	}
}

func TestParseApps(t *testing.T) {
	wrapped := `{"data":[{"id":"e35d","name":"file-transfer","srpName":"weda","version":1,"status":"ok"}]}`
	apps, err := parseApps([]byte(wrapped))
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 1 || apps[0].Name != "file-transfer" || apps[0].ID != "e35d" {
		t.Errorf("wrapped parse failed: %+v", apps)
	}

	plain := `[{"id":"1","name":"iam"}]`
	apps, err = parseApps([]byte(plain))
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 1 || apps[0].Name != "iam" {
		t.Errorf("plain parse failed: %+v", apps)
	}

	if _, err := parseApps([]byte("404 page not found")); err == nil {
		t.Error("expected error for non-JSON body")
	}
}

func TestPrettyJSON(t *testing.T) {
	out := prettyJSON([]byte(`{"b":1,"a":"x"}`))
	if out == `{"b":1,"a":"x"}` {
		t.Error("expected indented output")
	}
	// 非 JSON 原樣返回
	if prettyJSON([]byte("plain text")) != "plain text" {
		t.Error("non-JSON should pass through")
	}
}
