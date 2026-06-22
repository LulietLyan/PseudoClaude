package subagent

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

type parseFrontmatter struct {
	Name            string   `yaml:"name"`
	Description     string   `yaml:"description"`
	Tools           []string `yaml:"tools"`
	DisallowedTools []string `yaml:"disallowedTools"`
	Model           string   `yaml:"model"`
	MaxTurns        int      `yaml:"maxTurns"`
	PermissionMode  string   `yaml:"permissionMode"`
	Background      bool     `yaml:"background"`
}

func ParseDefinition(path string, source Source, data []byte) (Definition, error) {
	front, body, err := splitFrontmatter(string(data))
	if err != nil {
		return Definition{}, err
	}
	var fm parseFrontmatter
	if err := yaml.Unmarshal([]byte(front), &fm); err != nil {
		return Definition{}, fmt.Errorf("parse frontmatter: %w", err)
	}
	def := Definition{
		Name:            strings.TrimSpace(fm.Name),
		Description:     strings.TrimSpace(fm.Description),
		Tools:           cleanToolList(fm.Tools),
		DisallowedTools: cleanToolList(fm.DisallowedTools),
		MaxTurns:        fm.MaxTurns,
		Background:      fm.Background,
		SystemPrompt:    strings.TrimSpace(body),
		Source:          source,
		Path:            path,
	}
	if def.Name == "" {
		return Definition{}, errors.New("name is required")
	}
	if !namePattern.MatchString(def.Name) {
		return Definition{}, fmt.Errorf("invalid name %q", def.Name)
	}
	if def.Description == "" {
		return Definition{}, errors.New("description is required")
	}
	model, warning := ParseModelRef(fm.Model)
	def.Model = model
	if !warning.Empty() {
		def.Warnings = append(def.Warnings, warning.WithContext(path, def.Name))
	}
	perm, warning := ParsePermissionRef(fm.PermissionMode)
	def.Permission = perm
	if !warning.Empty() {
		def.Warnings = append(def.Warnings, warning.WithContext(path, def.Name))
	}
	if def.MaxTurns < 0 {
		def.MaxTurns = 0
		def.Warnings = append(def.Warnings, Warning{
			Path:    path,
			Agent:   def.Name,
			Field:   "maxTurns",
			Message: "maxTurns must be non-negative, falling back to 0",
		})
	}
	return def, nil
}

func splitFrontmatter(content string) (string, string, error) {
	content = strings.TrimPrefix(content, "\ufeff")
	if !strings.HasPrefix(content, "---") {
		return "", "", errors.New("missing frontmatter")
	}
	if len(content) > 3 {
		next := content[3]
		if next != '\n' && next != '\r' {
			return "", "", errors.New("missing frontmatter")
		}
	}
	rest := content[3:]
	rest = strings.TrimPrefix(rest, "\r\n")
	rest = strings.TrimPrefix(rest, "\n")
	for _, marker := range []string{"\n---\n", "\n---\r\n"} {
		if idx := strings.Index(rest, marker); idx >= 0 {
			return rest[:idx], rest[idx+len(marker):], nil
		}
	}
	if strings.HasSuffix(rest, "\n---") {
		return strings.TrimSuffix(rest, "\n---"), "", nil
	}
	return "", "", errors.New("unterminated frontmatter")
}

func cleanToolList(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
