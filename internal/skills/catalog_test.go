package skills

import (
	"os"
	"path/filepath"
	"testing"
)

type fakeToolLookup map[string]bool

func (f fakeToolLookup) IsKnown(name string) bool { return f[name] }

func TestLoadCatalogBuiltinOnly(t *testing.T) {
	catalog := LoadCatalog(LoadOptions{WorkDir: t.TempDir(), HomeDir: t.TempDir()})
	names := skillNames(catalog.List())
	want := []string{"commit", "review", "test"}
	if len(names) != len(want) {
		t.Fatalf("names = %+v warnings=%+v", names, catalog.Warnings())
	}
	for i, name := range want {
		if names[i] != name {
			t.Fatalf("names[%d] = %q want %q", i, names[i], name)
		}
	}
}

func TestLoadCatalogUserOverridesBuiltin(t *testing.T) {
	home := t.TempDir()
	writeSkill(t, filepath.Join(home, ".PseudoClaude", "skills", "commit", "SKILL.md"), "commit", "User commit.", "User body")
	catalog := LoadCatalog(LoadOptions{WorkDir: t.TempDir(), HomeDir: home})
	skill, ok := catalog.Get("commit")
	if !ok || skill.Source != SourceUser || skill.Meta.Description != "User commit." {
		t.Fatalf("skill = %+v ok=%v", skill, ok)
	}
}

func TestLoadCatalogProjectOverridesUser(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	writeSkill(t, filepath.Join(home, ".PseudoClaude", "skills", "demo", "SKILL.md"), "demo", "User demo.", "User body")
	writeSkill(t, filepath.Join(work, ".PseudoClaude", "skills", "demo", "SKILL.md"), "demo", "Project demo.", "Project body")
	catalog := LoadCatalog(LoadOptions{WorkDir: work, HomeDir: home})
	skill, ok := catalog.Get("demo")
	if !ok || skill.Source != SourceProject || skill.Meta.Description != "Project demo." {
		t.Fatalf("skill = %+v ok=%v", skill, ok)
	}
}

func TestLoadCatalogSkipsBrokenSkill(t *testing.T) {
	work := t.TempDir()
	if err := os.MkdirAll(filepath.Join(work, ".PseudoClaude", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, ".PseudoClaude", "skills", "broken.md"), []byte("broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, filepath.Join(work, ".PseudoClaude", "skills", "good.md"), "good", "Good.", "Body")
	catalog := LoadCatalog(LoadOptions{WorkDir: work, HomeDir: t.TempDir()})
	if _, ok := catalog.Get("good"); !ok {
		t.Fatal("good skill missing")
	}
	if len(catalog.Warnings()) == 0 {
		t.Fatal("expected warning")
	}
}

func TestValidateToolsMissing(t *testing.T) {
	catalog := LoadCatalog(LoadOptions{WorkDir: t.TempDir(), HomeDir: t.TempDir()})
	warnings := catalog.ValidateTools(fakeToolLookup{"read_file": true}, map[string]bool{"load_skill": true})
	if len(warnings) == 0 {
		t.Fatal("expected missing tool warnings")
	}
}

func TestValidateToolsOwnToolSpec(t *testing.T) {
	work := t.TempDir()
	dir := filepath.Join(work, ".PseudoClaude", "skills", "demo")
	writeSkill(t, filepath.Join(dir, "SKILL.md"), "demo", "Demo.", "Body")
	data := `{"tools":[{"name":"own_tool","command":["tool.sh"]}]}`
	if err := os.WriteFile(filepath.Join(dir, "tools.json"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: demo\ndescription: Demo.\ntools:\n  - own_tool\n---\nBody"), 0o644); err != nil {
		t.Fatal(err)
	}
	catalog := LoadCatalog(LoadOptions{WorkDir: work, HomeDir: t.TempDir()})
	warnings := catalog.ValidateTools(fakeToolLookup{
		"read_file": true, "search_code": true, "find_files": true, "run_command": true,
	}, nil)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %+v", warnings)
	}
}

func writeSkill(t *testing.T, path, name, description, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data := "---\nname: " + name + "\ndescription: " + description + "\n---\n" + body
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func skillNames(skills []Skill) []string {
	out := make([]string, 0, len(skills))
	for _, skill := range skills {
		out = append(out, skill.Meta.Name)
	}
	return out
}
