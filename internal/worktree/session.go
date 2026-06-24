package worktree

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

func loadSession(path string) (*Session, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 || string(data) == "null" {
		return nil, nil
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	if s.WorktreePath == "" {
		return nil, nil
	}
	return &s, nil
}

func saveSession(path string, s *Session) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data := []byte("null\n")
	if s != nil {
		var err error
		data, err = json.MarshalIndent(s, "", "  ")
		if err != nil {
			return err
		}
		data = append(data, '\n')
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func clearSession(path string) error {
	return saveSession(path, nil)
}
