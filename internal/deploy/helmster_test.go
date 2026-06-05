package deploy

import "testing"

func TestBasenameOf(t *testing.T) {
	cases := map[string]string{
		"registry.example.com/proj/file-transfer":      "file-transfer",
		"registry.example.com/proj/weda_file_transfer": "weda_file_transfer",
		"file-transfer": "file-transfer",
		"":              "",
	}
	for in, want := range cases {
		if got := basenameOf(in); got != want {
			t.Errorf("basenameOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSetQ(t *testing.T) {
	// 一般值
	if got, want := setQ("global.tenantId", "tid-1234"), "--set global.tenantId='tid-1234'"; got != want {
		t.Errorf("setQ plain = %q, want %q", got, want)
	}
	// 含單引號的值
	if got, want := setQ("k", "it's"), `--set k='it'\''s'`; got != want {
		t.Errorf("setQ quoted = %q, want %q", got, want)
	}
}
