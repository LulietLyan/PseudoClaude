package instructions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoaderLoadsLayersAndIncludes(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "rules.md"), []byte("root rules"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, FileName), []byte("root\n@include rules.md"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".PseudoClaude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".PseudoClaude", FileName), []byte("project config"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".PseudoClaude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".PseudoClaude", FileName), []byte("user config"), 0o644); err != nil {
		t.Fatal(err)
	}

	loader := Loader{ProjectRoot: root, UserHome: home, MaxDepth: DefaultMaxDepth}
	result := loader.Load()
	content := result.Content
	for _, want := range []string{"root rules", "project config", "user config"} {
		if !strings.Contains(content, want) {
			t.Fatalf("content missing %q:\n%s", want, content)
		}
	}
	if !(strings.Index(content, "root") < strings.Index(content, "project config") && strings.Index(content, "project config") < strings.Index(content, "user config")) {
		t.Fatalf("layers not in priority order:\n%s", content)
	}
}

func TestIncludeSafetyWarnings(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.md"), []byte("@include b.md"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.md"), []byte("@include a.md"), 0o644); err != nil {
		t.Fatal(err)
	}
	content, warnings := (expander{maxDepth: 5}).expand(filepath.Join(root, "a.md"), root, 0, map[string]struct{}{})
	if !strings.Contains(content, "检测到环路") || len(warnings) == 0 {
		t.Fatalf("missing cycle warning: content=%q warnings=%v", content, warnings)
	}

	if rel, ok := isIncludeLine("please @include a.md"); ok || rel != "" {
		t.Fatalf("non-exclusive include matched: %q %v", rel, ok)
	}
	if _, ok := isIncludeLine("@include /tmp/outside.md"); ok {
		t.Fatal("absolute include should be rejected")
	}
}
