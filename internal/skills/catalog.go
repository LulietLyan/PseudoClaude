package skills

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

//go:embed builtin/*
var BuiltinFS embed.FS

type Catalog struct {
	mu       sync.RWMutex
	byName   map[string]Skill
	warnings []Warning
}

type LoadOptions struct {
	WorkDir    string
	HomeDir    string
	BuiltinDir string
	UserDir    string
	ProjectDir string
}

type Warning struct {
	Source string
	Path   string
	Skill  string
	Reason string
}

type ReloadResult struct {
	Added    []string
	Removed  []string
	Updated  []string
	Warnings []Warning
}

type PromptItem struct {
	Name        string
	Description string
}

type Summary struct {
	Name        string
	Description string
	Source      Source
	Mode        ExecutionMode
}

type ToolLookup interface {
	IsKnown(name string) bool
}

func LoadCatalog(opts LoadOptions) *Catalog {
	c := &Catalog{byName: make(map[string]Skill)}
	c.load(opts)
	return c
}

func (c *Catalog) Reload(opts LoadOptions) ReloadResult {
	next := LoadCatalog(opts)
	c.mu.Lock()
	defer c.mu.Unlock()
	old := c.byName
	c.byName = next.byName
	c.warnings = next.warnings
	return diffCatalog(old, next.byName, next.warnings)
}

func (c *Catalog) Get(name string) (Skill, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	skill, ok := c.byName[name]
	return skill, ok
}

func (c *Catalog) List() []Skill {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]Skill, 0, len(c.byName))
	for _, skill := range c.byName {
		out = append(out, skill)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Meta.Name < out[j].Meta.Name })
	return out
}

func (c *Catalog) Remove(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.byName, name)
}

func (c *Catalog) Warnings() []Warning {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]Warning(nil), c.warnings...)
}

func (c *Catalog) ValidateTools(reg ToolLookup, protected map[string]bool) []Warning {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var warnings []Warning
	for _, skill := range c.byName {
		own := make(map[string]bool, len(skill.Tools))
		for _, spec := range skill.Tools {
			own[spec.Name] = true
		}
		for _, name := range skill.Meta.Tools {
			if protected[name] || own[name] || (reg != nil && reg.IsKnown(name)) {
				continue
			}
			warnings = append(warnings, Warning{
				Source: string(skill.Source),
				Path:   skill.EntryPath,
				Skill:  skill.Meta.Name,
				Reason: fmt.Sprintf("tool %q is not registered", name),
			})
			break
		}
	}
	sortWarnings(warnings)
	return warnings
}

func (c *Catalog) PromptItems() []PromptItem {
	skills := c.List()
	out := make([]PromptItem, 0, len(skills))
	for _, skill := range skills {
		out = append(out, PromptItem{Name: skill.Meta.Name, Description: skill.Meta.Description})
	}
	return out
}

func (c *Catalog) Summaries() []Summary {
	skills := c.List()
	out := make([]Summary, 0, len(skills))
	for _, skill := range skills {
		out = append(out, Summary{
			Name:        skill.Meta.Name,
			Description: skill.Meta.Description,
			Source:      skill.Source,
			Mode:        skill.Meta.Mode,
		})
	}
	return out
}

func (c *Catalog) load(opts LoadOptions) {
	opts = normalizeOptions(opts)
	c.scanBuiltin(opts.BuiltinDir)
	c.scanDir(opts.UserDir, SourceUser)
	c.scanDir(opts.ProjectDir, SourceProject)
}

func normalizeOptions(opts LoadOptions) LoadOptions {
	if opts.WorkDir == "" {
		opts.WorkDir, _ = os.Getwd()
	}
	if opts.HomeDir == "" {
		opts.HomeDir, _ = os.UserHomeDir()
	}
	if opts.UserDir == "" && opts.HomeDir != "" {
		opts.UserDir = filepath.Join(opts.HomeDir, ".PseudoClaude", "skills")
	}
	if opts.ProjectDir == "" && opts.WorkDir != "" {
		opts.ProjectDir = filepath.Join(opts.WorkDir, ".PseudoClaude", "skills")
	}
	if opts.BuiltinDir == "" {
		opts.BuiltinDir = "builtin"
	}
	return opts
}

func (c *Catalog) scanBuiltin(root string) {
	entries, err := BuiltinFS.ReadDir(root)
	if err != nil {
		c.warn(Warning{Source: string(SourceBuiltin), Path: root, Reason: err.Error()})
		return
	}
	tmp, err := os.MkdirTemp("", "pseudoclaude-builtin-skills-*")
	if err != nil {
		c.warn(Warning{Source: string(SourceBuiltin), Path: root, Reason: err.Error()})
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if entry.IsDir() {
			if err := copyEmbedDir(BuiltinFS, filepath.Join(root, name), filepath.Join(tmp, name)); err != nil {
				c.warn(Warning{Source: string(SourceBuiltin), Path: filepath.Join(root, name), Reason: err.Error()})
				continue
			}
			c.addCandidate(filepath.Join(tmp, name), SourceBuiltin, true)
			continue
		}
		if strings.HasSuffix(strings.ToLower(name), ".md") {
			data, err := BuiltinFS.ReadFile(filepath.Join(root, name))
			if err != nil {
				c.warn(Warning{Source: string(SourceBuiltin), Path: filepath.Join(root, name), Reason: err.Error()})
				continue
			}
			target := filepath.Join(tmp, name)
			if err := os.WriteFile(target, data, 0o644); err != nil {
				c.warn(Warning{Source: string(SourceBuiltin), Path: filepath.Join(root, name), Reason: err.Error()})
				continue
			}
			c.addCandidate(target, SourceBuiltin, false)
		}
	}
}

func (c *Catalog) scanDir(root string, source Source) {
	if strings.TrimSpace(root) == "" {
		return
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if !os.IsNotExist(err) {
			c.warn(Warning{Source: string(source), Path: root, Reason: err.Error()})
		}
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		path := filepath.Join(root, name)
		if entry.IsDir() {
			c.addCandidate(path, source, true)
		} else if strings.HasSuffix(strings.ToLower(name), ".md") {
			c.addCandidate(path, source, false)
		}
	}
}

func (c *Catalog) addCandidate(path string, source Source, dir bool) {
	var (
		skill Skill
		err   error
	)
	if dir {
		skill, err = ParseDir(path, source)
	} else {
		skill, err = ParseFile(path, source)
	}
	if err != nil {
		c.warn(Warning{Source: string(source), Path: path, Reason: err.Error()})
		return
	}
	c.byName[skill.Meta.Name] = skill
}

func (c *Catalog) warn(w Warning) {
	c.warnings = append(c.warnings, w)
}

func diffCatalog(old, next map[string]Skill, warnings []Warning) ReloadResult {
	var out ReloadResult
	for name, skill := range next {
		prev, ok := old[name]
		if !ok {
			out.Added = append(out.Added, name)
			continue
		}
		if prev.EntryPath != skill.EntryPath || prev.Meta.Description != skill.Meta.Description || prev.Source != skill.Source {
			out.Updated = append(out.Updated, name)
		}
	}
	for name := range old {
		if _, ok := next[name]; !ok {
			out.Removed = append(out.Removed, name)
		}
	}
	sort.Strings(out.Added)
	sort.Strings(out.Removed)
	sort.Strings(out.Updated)
	out.Warnings = append([]Warning(nil), warnings...)
	sortWarnings(out.Warnings)
	return out
}

func sortWarnings(warnings []Warning) {
	sort.Slice(warnings, func(i, j int) bool {
		if warnings[i].Skill != warnings[j].Skill {
			return warnings[i].Skill < warnings[j].Skill
		}
		return warnings[i].Path < warnings[j].Path
	})
}

func copyEmbedDir(fsys embed.FS, src, dst string) error {
	entries, err := fsys.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		from := filepath.Join(src, entry.Name())
		to := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := copyEmbedDir(fsys, from, to); err != nil {
				return err
			}
			continue
		}
		data, err := fsys.ReadFile(from)
		if err != nil {
			return err
		}
		if err := os.WriteFile(to, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}
