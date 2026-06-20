package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitMarkdownMinimal(t *testing.T) {
	meta, body, err := SplitMarkdown([]byte("---\nname: demo\ndescription: Do one useful thing.\n---\n\nBody"))
	if err != nil {
		t.Fatal(err)
	}
	if meta.Name != "demo" || meta.Description == "" || meta.Mode != ModeShared || meta.History != HistoryRecent {
		t.Fatalf("meta = %+v", meta)
	}
	if body != "Body" {
		t.Fatalf("body = %q", body)
	}
}

func TestSplitMarkdownMissingFrontmatter(t *testing.T) {
	if _, _, err := SplitMarkdown([]byte("name: demo")); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseFileInvalidName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.md")
	if err := os.WriteFile(path, []byte("---\nname: Bad Name\ndescription: Bad.\n---\nBody"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseFile(path, SourceUser); err == nil || !strings.Contains(err.Error(), "invalid skill name") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseDirSkillMD(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: demo\ndescription: Demo.\n---\nBody"), 0o644); err != nil {
		t.Fatal(err)
	}
	skill, err := ParseDir(dir, SourceProject)
	if err != nil {
		t.Fatal(err)
	}
	if skill.Meta.Name != "demo" || skill.RootDir != dir || skill.Source != SourceProject {
		t.Fatalf("skill = %+v", skill)
	}
}

func TestParseToolsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tools.json")
	data := `{"tools":[{"name":"parse_resume","description":"Parse.","input_schema":{"type":"object"},"command":["references/parse.sh"]}]}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	specs, err := ParseToolsFile(path, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 || specs[0].Name != "parse_resume" || specs[0].RootDir != dir {
		t.Fatalf("specs = %+v", specs)
	}
}

func TestParseToolsFileRejectsEscape(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tools.json")
	data := `{"tools":[{"name":"bad","command":["../bad.sh"]}]}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseToolsFile(path, dir); err == nil {
		t.Fatal("expected escape error")
	}
}
