package subagent

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

//go:embed builtin/*.md
var builtinFiles embed.FS

type Catalog struct {
	mu       sync.RWMutex
	active   map[string]Definition
	all      map[string][]Definition
	warnings []Warning
}

type LoadOptions struct {
	ProjectRoot string
	HomeDir     string
	Logf        func(format string, args ...any)
}

type ReloadResult struct {
	Warnings []Warning
	Count    int
}

func LoadCatalog(opts LoadOptions) *Catalog {
	c := &Catalog{}
	c.reload(opts)
	return c
}

func (c *Catalog) Resolve(name string) (Definition, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	def, ok := c.active[name]
	return cloneDefinition(def), ok
}

func (c *Catalog) List() []Definition {
	c.mu.RLock()
	defer c.mu.RUnlock()
	defs := make([]Definition, 0, len(c.active))
	for _, def := range c.active {
		defs = append(defs, cloneDefinition(def))
	}
	sortDefinitions(defs)
	return defs
}

func (c *Catalog) ListAll(name string) []Definition {
	c.mu.RLock()
	defer c.mu.RUnlock()
	defs := append([]Definition(nil), c.all[name]...)
	for i := range defs {
		defs[i] = cloneDefinition(defs[i])
	}
	sort.Slice(defs, func(i, j int) bool {
		if defs[i].Source.Priority() == defs[j].Source.Priority() {
			return defs[i].Path < defs[j].Path
		}
		return defs[i].Source.Priority() > defs[j].Source.Priority()
	})
	return defs
}

func (c *Catalog) Warnings() []Warning {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]Warning(nil), c.warnings...)
}

func (c *Catalog) Reload(opts LoadOptions) ReloadResult {
	c.reload(opts)
	return ReloadResult{Warnings: c.Warnings(), Count: len(c.List())}
}

func (c *Catalog) reload(opts LoadOptions) {
	active := map[string]Definition{}
	all := map[string][]Definition{}
	var warnings []Warning
	add := func(def Definition) {
		all[def.Name] = append(all[def.Name], cloneDefinition(def))
		if cur, ok := active[def.Name]; !ok || def.Source.Priority() >= cur.Source.Priority() {
			active[def.Name] = cloneDefinition(def)
		}
		warnings = append(warnings, def.Warnings...)
	}
	for _, def := range loadPluginDefinitions() {
		add(def)
	}
	for _, def := range mustLoadBuiltinDefinitions() {
		add(def)
	}
	for _, def := range loadDirDefinitions(filepath.Join(opts.HomeDir, ".PseudoClaude", "agents"), SourceUser, &warnings) {
		add(def)
	}
	for _, def := range loadDirDefinitions(filepath.Join(opts.ProjectRoot, ".PseudoClaude", "agents"), SourceProject, &warnings) {
		add(def)
	}
	c.mu.Lock()
	c.active = active
	c.all = all
	c.warnings = warnings
	c.mu.Unlock()
	for _, warning := range warnings {
		if opts.Logf != nil {
			opts.Logf("subagent warning: %s", FormatWarning(warning))
		}
	}
}

func loadPluginDefinitions() []Definition {
	return nil
}

func mustLoadBuiltinDefinitions() []Definition {
	entries, err := builtinFiles.ReadDir("builtin")
	if err != nil {
		panic(fmt.Sprintf("read builtin subagents: %v", err))
	}
	defs := make([]Definition, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		path := filepath.Join("builtin", entry.Name())
		data, err := builtinFiles.ReadFile(path)
		if err != nil {
			panic(fmt.Sprintf("read builtin subagent %s: %v", path, err))
		}
		def, err := ParseDefinition(path, SourceBuiltin, data)
		if err != nil {
			panic(fmt.Sprintf("parse builtin subagent %s: %v", path, err))
		}
		defs = append(defs, def)
	}
	sortDefinitions(defs)
	return defs
}

func loadDirDefinitions(dir string, source Source, warnings *[]Warning) []Definition {
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		*warnings = append(*warnings, Warning{Path: dir, Message: err.Error()})
		return nil
	}
	defs := make([]Definition, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			*warnings = append(*warnings, Warning{Path: path, Message: err.Error()})
			continue
		}
		def, err := ParseDefinition(path, source, data)
		if err != nil {
			*warnings = append(*warnings, Warning{Path: path, Message: err.Error()})
			continue
		}
		defs = append(defs, def)
	}
	sortDefinitions(defs)
	return defs
}

func sortDefinitions(defs []Definition) {
	sort.Slice(defs, func(i, j int) bool {
		if defs[i].Name == defs[j].Name {
			return defs[i].Source.Priority() > defs[j].Source.Priority()
		}
		return defs[i].Name < defs[j].Name
	})
}

func cloneDefinition(def Definition) Definition {
	def.Tools = append([]string(nil), def.Tools...)
	def.DisallowedTools = append([]string(nil), def.DisallowedTools...)
	def.Warnings = append([]Warning(nil), def.Warnings...)
	return def
}
