package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"PseudoClaude/internal/skills"
)

func TestLoadSkillActivatesSkill(t *testing.T) {
	work := t.TempDir()
	dir := filepath.Join(work, ".PseudoClaude", "skills", "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: demo\ndescription: Demo.\n---\nFull SOP"), 0o644); err != nil {
		t.Fatal(err)
	}
	catalog := skills.LoadCatalog(skills.LoadOptions{WorkDir: work, HomeDir: t.TempDir()})
	active := skills.NewActiveSkills()
	registry, _ := NewRegistry()
	tool := NewLoadSkillTool(catalog, active, registry)
	result := tool.Execute(context.Background(), json.RawMessage(`{"name":"demo"}`), DefaultEnv(work))
	if !result.OK {
		t.Fatalf("result = %+v", result)
	}
	snapshot := active.Snapshot()
	if len(snapshot) != 1 || snapshot[0].Name != "demo" || !strings.Contains(snapshot[0].Body, "Full SOP") {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

func TestLoadSkillUnknown(t *testing.T) {
	catalog := skills.LoadCatalog(skills.LoadOptions{WorkDir: t.TempDir(), HomeDir: t.TempDir()})
	tool := NewLoadSkillTool(catalog, skills.NewActiveSkills(), nil)
	result := tool.Execute(context.Background(), json.RawMessage(`{"name":"missing"}`), DefaultEnv(t.TempDir()))
	if result.OK || result.ErrorType != "unknown_skill" {
		t.Fatalf("result = %+v", result)
	}
}
