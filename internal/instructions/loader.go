package instructions

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	FileName        = "PSEUDOCLAUDE.md"
	DefaultMaxDepth = 5
)

type Layer struct {
	Name     string
	Path     string
	Boundary string
}

type LoadResult struct {
	Content  string
	Loaded   []string
	Warnings []string
}

type Loader struct {
	ProjectRoot string
	UserHome    string
	MaxDepth    int
}

func NewLoader(projectRoot string) Loader {
	home, _ := os.UserHomeDir()
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		root = projectRoot
	}
	return Loader{ProjectRoot: root, UserHome: home, MaxDepth: DefaultMaxDepth}
}

func (l Loader) Layers() []Layer {
	maxDepth := l.MaxDepth
	if maxDepth <= 0 {
		maxDepth = DefaultMaxDepth
	}
	_ = maxDepth
	return []Layer{
		{Name: "project-root", Path: filepath.Join(l.ProjectRoot, FileName), Boundary: l.ProjectRoot},
		{Name: "project-config", Path: filepath.Join(l.ProjectRoot, ".PseudoClaude", FileName), Boundary: l.ProjectRoot},
		{Name: "user", Path: filepath.Join(l.UserHome, ".PseudoClaude", FileName), Boundary: filepath.Join(l.UserHome, ".PseudoClaude")},
	}
}

func (l Loader) Load() LoadResult {
	maxDepth := l.MaxDepth
	if maxDepth <= 0 {
		maxDepth = DefaultMaxDepth
	}
	exp := expander{maxDepth: maxDepth}
	var result LoadResult
	var parts []string
	for _, layer := range l.Layers() {
		if _, err := os.Stat(layer.Path); os.IsNotExist(err) {
			continue
		} else if err != nil {
			result.Warnings = append(result.Warnings, err.Error())
			continue
		}
		content, warnings := exp.expand(layer.Path, layer.Boundary, 0, map[string]struct{}{})
		result.Warnings = append(result.Warnings, warnings...)
		result.Loaded = append(result.Loaded, layer.Path)
		parts = append(parts, fmt.Sprintf("## Source: %s (%s)\n\n%s", layer.Name, layer.Path, strings.TrimSpace(content)))
	}
	result.Content = strings.TrimSpace(strings.Join(parts, "\n\n---\n\n"))
	return result
}
