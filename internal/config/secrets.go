package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// 機密（如 FEED_PAT）存在獨立的 secrets.json（0600），不混進可分享的 state.json。
type secretsFile struct {
	Secrets map[string]string `json:"secrets"`
}

func (s *Store) secretsPath() string {
	return filepath.Join(filepath.Dir(s.path), "secrets.json")
}

func (s *Store) loadSecrets() (map[string]string, error) {
	b, err := os.ReadFile(s.secretsPath())
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var sf secretsFile
	if err := json.Unmarshal(b, &sf); err != nil {
		return nil, err
	}
	if sf.Secrets == nil {
		sf.Secrets = map[string]string{}
	}
	return sf.Secrets, nil
}

func (s *Store) GetSecret(key string) (string, error) {
	m, err := s.loadSecrets()
	if err != nil {
		return "", err
	}
	return m[key], nil
}

func (s *Store) SetSecret(key, val string) error {
	m, err := s.loadSecrets()
	if err != nil {
		return err
	}
	if val == "" {
		delete(m, key)
	} else {
		m[key] = val
	}
	b, err := json.MarshalIndent(secretsFile{Secrets: m}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.secretsPath(), b, 0o600)
}
