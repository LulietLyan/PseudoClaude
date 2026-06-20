package skills

import (
	"fmt"
	"regexp"
	"strings"
)

type ExecutionMode string

const (
	ModeShared   ExecutionMode = "shared"
	ModeIsolated ExecutionMode = "isolated"
)

type HistoryMode string

const (
	HistoryNone    HistoryMode = "none"
	HistoryRecent  HistoryMode = "recent"
	HistorySummary HistoryMode = "summary"
)

type Source string

const (
	SourceBuiltin Source = "builtin"
	SourceUser    Source = "user"
	SourceProject Source = "project"
)

type SkillMeta struct {
	Name        string        `yaml:"name"`
	Description string        `yaml:"description"`
	Tools       []string      `yaml:"tools,omitempty"`
	Mode        ExecutionMode `yaml:"mode,omitempty"`
	History     HistoryMode   `yaml:"history,omitempty"`
	Model       string        `yaml:"model,omitempty"`
}

type Skill struct {
	Meta      SkillMeta
	Body      string
	EntryPath string
	RootDir   string
	Source    Source
	Tools     []ToolSpec
}

type ToolSpec struct {
	Name        string
	Description string
	InputSchema map[string]any
	Command     []string
	RootDir     string
}

var (
	skillNameRE = regexp.MustCompile(`^[a-z0-9_][a-z0-9_-]{0,63}$`)
	toolNameRE  = regexp.MustCompile(`^[A-Za-z0-9_:.@/-]+$`)
)

func NormalizeMeta(meta SkillMeta) (SkillMeta, error) {
	meta.Name = strings.TrimSpace(meta.Name)
	meta.Description = strings.TrimSpace(meta.Description)
	meta.Model = strings.TrimSpace(meta.Model)
	meta.Tools = cleanStrings(meta.Tools)
	if err := ValidateName(meta.Name); err != nil {
		return meta, err
	}
	if meta.Description == "" {
		return meta, fmt.Errorf("description is required")
	}
	if meta.Mode == "" {
		meta.Mode = ModeShared
	}
	switch meta.Mode {
	case ModeShared, ModeIsolated:
	default:
		return meta, fmt.Errorf("invalid mode %q", meta.Mode)
	}
	if meta.History == "" {
		meta.History = HistoryRecent
	}
	switch meta.History {
	case HistoryNone, HistoryRecent, HistorySummary:
	default:
		return meta, fmt.Errorf("invalid history %q", meta.History)
	}
	return meta, nil
}

func ValidateName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if !skillNameRE.MatchString(name) || strings.HasSuffix(name, "-") {
		return fmt.Errorf("invalid skill name %q", name)
	}
	return nil
}

func ValidateToolName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("tool name is required")
	}
	if !toolNameRE.MatchString(name) {
		return fmt.Errorf("invalid tool name %q", name)
	}
	return nil
}

func cleanStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}
