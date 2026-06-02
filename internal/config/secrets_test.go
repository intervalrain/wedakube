package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSecretsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	s, err := DefaultStore()
	if err != nil {
		t.Fatalf("store: %v", err)
	}

	if v, _ := s.GetSecret("FEED_PAT"); v != "" {
		t.Fatalf("expected empty, got %q", v)
	}
	if err := s.SetSecret("FEED_PAT", "tok-123"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if v, _ := s.GetSecret("FEED_PAT"); v != "tok-123" {
		t.Fatalf("get = %q, want tok-123", v)
	}

	// 存在獨立的 secrets.json，0600
	p := filepath.Join(dir, ".k3sdeploy", "secrets.json")
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Fatalf("perm = %o, want 600", perm)
	}

	// 不應寫進 state.json
	if _, err := os.Stat(filepath.Join(dir, ".k3sdeploy", "state.json")); !os.IsNotExist(err) {
		t.Fatalf("state.json should not exist after secret-only write")
	}

	// 清空 = 刪除 key
	if err := s.SetSecret("FEED_PAT", ""); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if v, _ := s.GetSecret("FEED_PAT"); v != "" {
		t.Fatalf("after clear = %q, want empty", v)
	}
}
