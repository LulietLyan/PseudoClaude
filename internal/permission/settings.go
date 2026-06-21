package permission

import (
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Settings struct {
	DefaultMode string `yaml:"defaultMode"`
	Permissions struct {
		Allow []string `yaml:"allow"`
		Deny  []string `yaml:"deny"`
	} `yaml:"permissions"`
}

type Options struct {
	UserPath    string
	ProjectPath string
	LocalPath   string
}

type LoadIssue struct {
	Path    string
	Message string
}

func DefaultOptions(root string) Options {
	home, err := os.UserHomeDir()
	userPath := filepath.Join(".PseudoClaude", "permissions.yaml")
	if err == nil && home != "" {
		userPath = filepath.Join(home, ".PseudoClaude", "permissions.yaml")
	}
	return Options{
		UserPath:    userPath,
		ProjectPath: filepath.Join(root, ".PseudoClaude", "permissions.yaml"),
		LocalPath:   filepath.Join(root, ".PseudoClaude", "permissions.local.yaml"),
	}
}

func loadSettings(path string) (Settings, []LoadIssue) {
	var settings Settings
	if path == "" {
		return settings, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return settings, nil
		}
		return Settings{}, []LoadIssue{{Path: path, Message: err.Error()}}
	}
	if err := yaml.Unmarshal(data, &settings); err != nil {
		return Settings{}, []LoadIssue{{Path: path, Message: err.Error()}}
	}
	return settings, nil
}

func settingsToRuleSet(settings Settings) (RuleSet, []LoadIssue) {
	var rules RuleSet
	var issues []LoadIssue
	for _, text := range settings.Permissions.Allow {
		rule, err := parseRuleWithError(text, DecisionAllow)
		if err != nil {
			issues = append(issues, LoadIssue{Message: "rule \"" + text + "\" parse failed: " + err.Error()})
			continue
		}
		rules.Allow = append(rules.Allow, rule)
	}
	for _, text := range settings.Permissions.Deny {
		rule, err := parseRuleWithError(text, DecisionDeny)
		if err != nil {
			issues = append(issues, LoadIssue{Message: "rule \"" + text + "\" parse failed: " + err.Error()})
			continue
		}
		rules.Deny = append(rules.Deny, rule)
	}
	return rules, issues
}

func chooseStartMode(local, project, user Settings) Mode {
	for _, value := range []string{local.DefaultMode, project.DefaultMode, user.DefaultMode} {
		if value == "" {
			continue
		}
		mode := ParseMode(value)
		if mode.String() == value {
			return mode
		}
	}
	return ModeDefault
}
