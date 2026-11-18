package config

import (
	"path/filepath"
	"testing"
)

func tempStore(t *testing.T) *Store {
	t.Helper()
	return &Store{path: filepath.Join(t.TempDir(), "state.json")}
}

func TestCurrentHostRoundTrip(t *testing.T) {
	s := tempStore(t)

	cur, err := s.CurrentHost()
	if err != nil || cur != "" {
		t.Fatalf("empty store: got (%q, %v), want (\"\", nil)", cur, err)
	}

	if err := s.SetCurrentHost("dev"); err != nil {
		t.Fatal(err)
	}
	cur, err = s.CurrentHost()
	if err != nil || cur != "dev" {
		t.Fatalf("after set: got (%q, %v), want (\"dev\", nil)", cur, err)
	}
}

func TestDeleteHostClearsCurrent(t *testing.T) {
	s := tempStore(t)
	if err := s.PutHost(Host{Name: "dev"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCurrentHost("dev"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteHost("dev"); err != nil {
		t.Fatal(err)
	}
	cur, err := s.CurrentHost()
	if err != nil || cur != "" {
		t.Fatalf("after delete: got (%q, %v), want (\"\", nil)", cur, err)
	}
}

// auto mode needs to know which commit an image tag was built from, to diff it against the
// commit being deployed.
func TestBuildRevRoundTrip(t *testing.T) {
	s := tempStore(t)
	if _, ok, err := s.BuildRev("reg/proj/svc", "v1_20261118.1"); err != nil || ok {
		t.Fatalf("empty store: ok=%v err=%v, want not found", ok, err)
	}
	if err := s.RecordBuild("reg/proj/svc", "v1_20261118.1", "abc123"); err != nil {
		t.Fatal(err)
	}
	rev, ok, err := s.BuildRev("reg/proj/svc", "v1_20261118.1")
	if err != nil || !ok || rev != "abc123" {
		t.Fatalf("BuildRev = %q, %v, %v; want abc123", rev, ok, err)
	}
	if _, ok, _ := s.BuildRev("reg/proj/other", "v1_20261118.1"); ok {
		t.Error("builds are keyed by image repo and tag")
	}
}
