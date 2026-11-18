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
