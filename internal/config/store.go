package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// State 是落地在 ~/.k3sdeploy/state.json 的全部設定。
type State struct {
	Hosts       []Host              `json:"hosts"`                 // 受管主機
	CurrentHost string              `json:"currentHost,omitempty"` // CLI `kube ctx select` 選定的 host（Host.Name）
	Pins        map[string][]string `json:"pins"`                  // key = host.Name -> 服務名清單
	Targets     map[string]Target   `json:"targets"`               // key = repoPath
	Counters    map[string]int      `json:"counters"`              // key = service|YYYYMMDD -> 當日 build 序號
	Builds      map[string]string   `json:"builds,omitempty"`      // key = imageRepo:tag -> git commit it was built from
}

type Store struct{ path string }

// NewStore opens the store at path (tests, or an alternate state file).
func NewStore(path string) *Store { return &Store{path: path} }

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
	st := State{Pins: map[string][]string{}, Targets: map[string]Target{}, Counters: map[string]int{}, Builds: map[string]string{}}
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
	if st.Pins == nil {
		st.Pins = map[string][]string{}
	}
	if st.Targets == nil {
		st.Targets = map[string]Target{}
	}
	if st.Counters == nil {
		st.Counters = map[string]int{}
	}
	if st.Builds == nil {
		st.Builds = map[string]string{}
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

// --- Hosts ---

func (s *Store) ListHosts() ([]Host, error) {
	st, err := s.load()
	return st.Hosts, err
}

func (s *Store) GetHost(name string) (Host, bool, error) {
	st, err := s.load()
	if err != nil {
		return Host{}, false, err
	}
	for _, h := range st.Hosts {
		if h.Name == name {
			return h, true, nil
		}
	}
	return Host{}, false, nil
}

// PutHost 以 Name 為鍵做 upsert。
func (s *Store) PutHost(h Host) error {
	st, err := s.load()
	if err != nil {
		return err
	}
	for i := range st.Hosts {
		if st.Hosts[i].Name == h.Name {
			st.Hosts[i] = h
			return s.save(st)
		}
	}
	st.Hosts = append(st.Hosts, h)
	return s.save(st)
}

func (s *Store) DeleteHost(name string) error {
	st, err := s.load()
	if err != nil {
		return err
	}
	out := st.Hosts[:0]
	for _, h := range st.Hosts {
		if h.Name != name {
			out = append(out, h)
		}
	}
	st.Hosts = out
	delete(st.Pins, name)
	if st.CurrentHost == name {
		st.CurrentHost = ""
	}
	return s.save(st)
}

// CurrentHost 回傳 CLI 選定的 host 名稱（可能為空 = 未選）。
func (s *Store) CurrentHost() (string, error) {
	st, err := s.load()
	return st.CurrentHost, err
}

// SetCurrentHost 設定 CLI 的目前 host；不驗證存在與否，由呼叫端把關。
func (s *Store) SetCurrentHost(name string) error {
	st, err := s.load()
	if err != nil {
		return err
	}
	st.CurrentHost = name
	return s.save(st)
}

// --- Pins ---

func (s *Store) Pins(hostName string) ([]string, error) {
	st, err := s.load()
	if err != nil {
		return nil, err
	}
	return st.Pins[hostName], nil
}

func (s *Store) SetPin(hostName, service string, pinned bool) error {
	st, err := s.load()
	if err != nil {
		return err
	}
	cur := st.Pins[hostName]
	out := cur[:0]
	found := false
	for _, svc := range cur {
		if svc == service {
			found = true
			if pinned {
				out = append(out, svc)
			}
			continue
		}
		out = append(out, svc)
	}
	if pinned && !found {
		out = append(out, service)
	}
	st.Pins[hostName] = out
	return s.save(st)
}

// --- Targets ---

func (s *Store) GetTarget(repoPath string) (Target, bool, error) {
	st, err := s.load()
	if err != nil {
		return Target{}, false, err
	}
	t, ok := st.Targets[repoPath]
	return t, ok, nil
}

// TargetsForHost 回傳屬於某 host 的部署目標（Host 相符，或舊版 SSHAlias 相符）。
func (s *Store) TargetsForHost(host string) ([]Target, error) {
	st, err := s.load()
	if err != nil {
		return nil, err
	}
	var out []Target
	for _, t := range st.Targets {
		if t.Host == host || (t.Host == "" && t.SSHAlias == host) {
			out = append(out, t)
		}
	}
	return out, nil
}

func (s *Store) DeleteTarget(repoPath string) error {
	st, err := s.load()
	if err != nil {
		return err
	}
	delete(st.Targets, repoPath)
	return s.save(st)
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

// RecordBuild remembers which commit an image tag was built from.
func (s *Store) RecordBuild(imageRepo, tag, rev string) error {
	st, err := s.load()
	if err != nil {
		return err
	}
	st.Builds[imageRepo+":"+tag] = rev
	return s.save(st)
}

// BuildRev returns the commit an image tag was built from, if kube built it.
func (s *Store) BuildRev(imageRepo, tag string) (string, bool, error) {
	st, err := s.load()
	if err != nil {
		return "", false, err
	}
	rev, ok := st.Builds[imageRepo+":"+tag]
	return rev, ok, nil
}
