package team

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

const (
	teamsDirName = "teams"
	configName   = "config.json"
	inboxDirName = "inboxes"
	tasksName    = "tasks.json"
)

func SanitizeName(name string) string {
	name = strings.TrimSpace(name)
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		ok := unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.'
		if ok {
			b.WriteRune(unicode.ToLower(r))
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func teamsRoot(homeDir string) string {
	return filepath.Join(homeDir, ".PseudoClaude", teamsDirName)
}

func uniqueSanitizedName(root, base string) (string, error) {
	if base == "" {
		return "", fmt.Errorf("team name is empty after sanitization")
	}
	name := base
	for i := 2; ; i++ {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			if os.IsNotExist(err) {
				return name, nil
			}
			return "", err
		}
		name = fmt.Sprintf("%s-%d", base, i)
	}
}

func derivePaths(t *Team, homeDir string) {
	t.ConfigDir = filepath.Join(teamsRoot(homeDir), t.SanitizedName)
	t.ConfigPath = filepath.Join(t.ConfigDir, configName)
	t.InboxDir = filepath.Join(t.ConfigDir, inboxDirName)
	t.TasksPath = filepath.Join(t.ConfigDir, tasksName)
}

func atomicWriteJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*.json")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	ok = true
	return nil
}

func loadTeam(path, homeDir string) (*Team, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var t Team
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	if t.SanitizedName == "" {
		t.SanitizedName = SanitizeName(t.Name)
	}
	if t.SanitizedName == "" || t.Name == "" {
		return nil, fmt.Errorf("invalid team config")
	}
	derivePaths(&t, homeDir)
	return &t, nil
}

func (t *Team) reloadMembers(homeDir string) error {
	latest, err := loadTeam(t.ConfigPath, homeDir)
	if err != nil {
		return err
	}
	t.Members = latest.Members
	return nil
}

func (t *Team) save() error {
	return atomicWriteJSON(t.ConfigPath, t)
}
