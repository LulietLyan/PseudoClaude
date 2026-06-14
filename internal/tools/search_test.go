package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindFiles(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.go"), "package a\n")
	mustWrite(t, filepath.Join(dir, "sub", "b.go"), "package b\n")
	mustWrite(t, filepath.Join(dir, "sub", "c.md"), "# c\n")
	env := DefaultEnv(dir)

	result := NewFindFilesTool().Execute(context.Background(), json.RawMessage(`{"pattern":"*.go"}`), env)
	if !result.OK || !strings.Contains(result.Content, "a.go") {
		t.Fatalf("glob result = %+v", result)
	}
	result = NewFindFilesTool().Execute(context.Background(), json.RawMessage(`{"pattern":"**/*.go"}`), env)
	if !result.OK || !strings.Contains(result.Content, "a.go") || !strings.Contains(result.Content, "b.go") {
		t.Fatalf("recursive result = %+v", result)
	}
	env.MaxSearchResults = 1
	result = NewFindFilesTool().Execute(context.Background(), json.RawMessage(`{"pattern":"**/*"}`), env)
	if !result.OK || result.Metadata["truncated"] != true {
		t.Fatalf("truncated result = %+v", result)
	}
}

func TestSearchCode(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "alpha\nbeta\n")
	mustWrite(t, filepath.Join(dir, "sub", "b.txt"), "gamma\nalphabet\n")
	env := DefaultEnv(dir)
	tool := NewSearchCodeTool()

	result := tool.Execute(context.Background(), json.RawMessage(`{"pattern":"alpha"}`), env)
	if !result.OK || !strings.Contains(result.Content, "a.txt:1") || !strings.Contains(result.Content, "b.txt:2") {
		t.Fatalf("text result = %+v", result)
	}
	result = tool.Execute(context.Background(), json.RawMessage(`{"pattern":"^beta$","regex":true}`), env)
	if !result.OK || !strings.Contains(result.Content, "a.txt:2") {
		t.Fatalf("regex result = %+v", result)
	}
	result = tool.Execute(context.Background(), json.RawMessage(`{"pattern":"[","regex":true}`), env)
	if result.OK || result.ErrorType != "invalid_arguments" {
		t.Fatalf("bad regex result = %+v", result)
	}
	env.MaxSearchResults = 1
	result = tool.Execute(context.Background(), json.RawMessage(`{"pattern":"a"}`), env)
	if !result.OK || result.Metadata["truncated"] != true {
		t.Fatalf("truncated result = %+v", result)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
