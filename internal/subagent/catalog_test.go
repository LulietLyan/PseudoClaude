package subagent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuiltinDefinitionsParse(t *testing.T) {
	catalog := LoadCatalog(LoadOptions{})
	for _, name := range []string{"general-purpose", "explore", "plan"} {
		def, ok := catalog.Resolve(name)
		if !ok {
			t.Fatalf("missing builtin %s", name)
		}
		if def.Description == "" || def.SystemPrompt == "" {
			t.Fatalf("builtin %s missing text: %#v", name, def)
		}
	}
}

func TestCatalogOverrideAndListAll(t *testing.T) {
	project := t.TempDir()
	home := t.TempDir()
	writeAgent(t, filepath.Join(home, ".PseudoClaude", "agents", "explore.md"), "explore", "User explore.")
	writeAgent(t, filepath.Join(project, ".PseudoClaude", "agents", "explore.md"), "explore", "Project explore.")
	catalog := LoadCatalog(LoadOptions{ProjectRoot: project, HomeDir: home})
	def, ok := catalog.Resolve("explore")
	if !ok {
		t.Fatal("missing explore")
	}
	if def.Source != SourceProject || def.Description != "Project explore." {
		t.Fatalf("override did not prefer project: %#v", def)
	}
	all := catalog.ListAll("explore")
	if len(all) < 3 {
		t.Fatalf("ListAll returned %d definitions, want builtin/user/project", len(all))
	}
	if all[0].Source != SourceProject {
		t.Fatalf("ListAll should put active highest priority first: %#v", all)
	}
}

func TestCatalogWarningsAndReload(t *testing.T) {
	project := t.TempDir()
	dir := filepath.Join(project, ".PseudoClaude", "agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.md"), []byte("---\nname: Bad\n---\nBody"), 0o644); err != nil {
		t.Fatal(err)
	}
	catalog := LoadCatalog(LoadOptions{ProjectRoot: project})
	if len(catalog.Warnings()) == 0 {
		t.Fatal("expected warning for invalid project role")
	}
	writeAgent(t, filepath.Join(dir, "demo.md"), "demo", "Demo role.")
	result := catalog.Reload(LoadOptions{ProjectRoot: project})
	if result.Count == 0 {
		t.Fatal("reload count should be non-zero")
	}
	if _, ok := catalog.Resolve("demo"); !ok {
		t.Fatal("reload did not load new role")
	}
}

func writeAgent(t *testing.T, path, name, description string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := `---
name: ` + name + `
description: ` + description + `
---
Prompt.
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
