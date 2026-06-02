package deploy

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// State 是落地在 ~/.k3sdeploy/state.json 的內容。
type State struct {
	Targets  map[string]Target `json:"targets"`  // key = repoPath
	Counters map[string]int    `json:"counters"` // key = service|YYYYMMDD -> 當日 build 序號
}

type Store struct{ path string }

func DefaultStore() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(home, ".k3sdeploy")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Store{path: filepath.Join(dir, "state.json")}, nil
}

func (s *Store) load() (State, error) {
	st := State{Targets: map[string]Target{}, Counters: map[string]int{}}
	b, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal(b, &st); err != nil {
		return st, err
	}
	if st.Targets == nil {
		st.Targets = map[string]Target{}
	}
	if st.Counters == nil {
		st.Counters = map[string]int{}
	}
	return st, nil
}

func (s *Store) save(st State) error {
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o600)
}

func (s *Store) GetTarget(repoPath string) (Target, bool, error) {
	st, err := s.load()
	if err != nil {
		return Target{}, false, err
	}
	t, ok := st.Targets[repoPath]
	return t, ok, nil
}

func (s *Store) PutTarget(t Target) error {
	st, err := s.load()
	if err != nil {
		return err
	}
	st.Targets[t.RepoPath] = t
	return s.save(st)
}

// NextBuildNumber 取得並遞增某服務某日的 build 序號（給 tag 的 .N 用）。
func (s *Store) NextBuildNumber(service, date string) (int, error) {
	st, err := s.load()
	if err != nil {
		return 0, err
	}
	key := service + "|" + date
	st.Counters[key]++
	n := st.Counters[key]
	if err := s.save(st); err != nil {
		return 0, err
	}
	return n, nil
}
