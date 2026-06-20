package skills

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func SplitMarkdown(data []byte) (SkillMeta, string, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") && strings.TrimSpace(text) != "---" {
		return SkillMeta{}, "", errors.New("missing YAML frontmatter")
	}
	lines := strings.Split(text, "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		return SkillMeta{}, "", errors.New("missing YAML frontmatter")
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return SkillMeta{}, "", errors.New("unterminated YAML frontmatter")
	}
	var meta SkillMeta
	if err := yaml.Unmarshal([]byte(strings.Join(lines[1:end], "\n")), &meta); err != nil {
		return SkillMeta{}, "", fmt.Errorf("parse frontmatter: %w", err)
	}
	meta, err := NormalizeMeta(meta)
	if err != nil {
		return SkillMeta{}, "", err
	}
	body := strings.TrimSpace(strings.Join(lines[end+1:], "\n"))
	if body == "" {
		return SkillMeta{}, "", errors.New("body is required")
	}
	return meta, body, nil
}

func ParseFile(path string, source Source) (Skill, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Skill{}, err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return Skill{}, err
	}
	meta, body, err := SplitMarkdown(data)
	if err != nil {
		return Skill{}, err
	}
	return Skill{
		Meta:      meta,
		Body:      body,
		EntryPath: abs,
		RootDir:   filepath.Dir(abs),
		Source:    source,
	}, nil
}

func ParseDir(dir string, source Source) (Skill, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Skill{}, err
	}
	entry := filepath.Join(abs, "SKILL.md")
	if _, err := os.Stat(entry); err != nil {
		if !os.IsNotExist(err) {
			return Skill{}, err
		}
		entry = filepath.Join(abs, filepath.Base(abs)+".md")
		if _, err := os.Stat(entry); err != nil {
			return Skill{}, fmt.Errorf("directory skill needs SKILL.md or %s: %w", filepath.Base(entry), err)
		}
	}
	skill, err := ParseFile(entry, source)
	if err != nil {
		return Skill{}, err
	}
	skill.RootDir = abs
	toolsPath := filepath.Join(abs, "tools.json")
	if _, err := os.Stat(toolsPath); err == nil {
		specs, err := ParseToolsFile(toolsPath, abs)
		if err != nil {
			return Skill{}, err
		}
		skill.Tools = specs
	} else if err != nil && !os.IsNotExist(err) {
		return Skill{}, err
	}
	return skill, nil
}

func ParseToolsFile(path string, root string) ([]ToolSpec, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Tools []struct {
			Name        string         `json:"name"`
			Description string         `json:"description"`
			InputSchema map[string]any `json:"input_schema"`
			Command     []string       `json:"command"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse tools.json: %w", err)
	}
	specs := make([]ToolSpec, 0, len(raw.Tools))
	for i, item := range raw.Tools {
		name := strings.TrimSpace(item.Name)
		if err := ValidateToolName(name); err != nil {
			return nil, fmt.Errorf("tools[%d]: %w", i, err)
		}
		if len(item.Command) == 0 {
			return nil, fmt.Errorf("tools[%d] %q: command is required", i, name)
		}
		command := append([]string(nil), item.Command...)
		if err := validateCommandPath(command[0], absRoot); err != nil {
			return nil, fmt.Errorf("tools[%d] %q: %w", i, name, err)
		}
		schema := item.InputSchema
		if schema == nil {
			schema = map[string]any{"type": "object"}
		}
		if typ, _ := schema["type"].(string); typ != "" && typ != "object" {
			return nil, fmt.Errorf("tools[%d] %q: input_schema type must be object", i, name)
		}
		specs = append(specs, ToolSpec{
			Name:        name,
			Description: strings.TrimSpace(item.Description),
			InputSchema: schema,
			Command:     command,
			RootDir:     absRoot,
		})
	}
	return specs, nil
}

func ReloadSkillBody(skill Skill) (Skill, error) {
	if strings.TrimSpace(skill.EntryPath) == "" {
		return skill, errors.New("entry path is empty")
	}
	next, err := ParseFile(skill.EntryPath, skill.Source)
	if err != nil {
		return skill, err
	}
	next.RootDir = skill.RootDir
	next.Tools = skill.Tools
	return next, nil
}

func validateCommandPath(first string, root string) error {
	first = strings.TrimSpace(first)
	if first == "" {
		return errors.New("command path is empty")
	}
	if filepath.IsAbs(first) {
		rel, err := filepath.Rel(root, filepath.Clean(first))
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return errors.New("absolute command escapes skill root")
		}
		return nil
	}
	cleaned := filepath.Clean(first)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return errors.New("relative command escapes skill root")
	}
	joined := filepath.Join(root, cleaned)
	rel, err := filepath.Rel(root, joined)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("relative command escapes skill root")
	}
	return nil
}
