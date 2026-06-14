package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := DefaultEnv(dir)
	tool := NewReadFileTool()
	result := tool.Execute(context.Background(), json.RawMessage(`{"path":"hello.txt"}`), env)
	if !result.OK || result.Content != "hello" {
		t.Fatalf("read result = %+v", result)
	}

	result = tool.Execute(context.Background(), json.RawMessage(`{"path":"missing.txt"}`), env)
	if result.OK || result.ErrorType != "not_found" {
		t.Fatalf("missing result = %+v", result)
	}
	result = tool.Execute(context.Background(), json.RawMessage(`{"path":"."}`), env)
	if result.OK || result.ErrorType != "invalid_path" {
		t.Fatalf("dir result = %+v", result)
	}
	if err := os.WriteFile(filepath.Join(dir, "bin"), []byte{0, 1, 2}, 0o644); err != nil {
		t.Fatal(err)
	}
	result = tool.Execute(context.Background(), json.RawMessage(`{"path":"bin"}`), env)
	if result.OK || result.ErrorType != "unsupported_content" {
		t.Fatalf("binary result = %+v", result)
	}
	env.MaxReadBytes = 4
	result = tool.Execute(context.Background(), json.RawMessage(`{"path":"hello.txt"}`), env)
	if !result.OK || result.Content != "hell" || result.Metadata["truncated"] != true {
		t.Fatalf("truncated result = %+v", result)
	}
}

func TestWriteFile(t *testing.T) {
	dir := t.TempDir()
	tool := NewWriteFileTool()
	result := tool.Execute(context.Background(), json.RawMessage(`{"path":"nested/out.txt","content":"hello"}`), DefaultEnv(dir))
	if !result.OK {
		t.Fatalf("write result = %+v", result)
	}
	data, err := os.ReadFile(filepath.Join(dir, "nested", "out.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("content = %q", data)
	}
	result = tool.Execute(context.Background(), json.RawMessage(`{"path":"x"}`), DefaultEnv(dir))
	if result.OK || result.ErrorType != "invalid_arguments" {
		t.Fatalf("missing content result = %+v", result)
	}
}

func TestEditFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("one two three"), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := NewEditFileTool()
	result := tool.Execute(context.Background(), json.RawMessage(`{"path":"file.txt","old_text":"two","new_text":"TWO"}`), DefaultEnv(dir))
	if !result.OK {
		t.Fatalf("edit result = %+v", result)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "one TWO three" {
		t.Fatalf("content = %q", data)
	}

	before := string(data)
	result = tool.Execute(context.Background(), json.RawMessage(`{"path":"file.txt","old_text":"missing","new_text":"x"}`), DefaultEnv(dir))
	if result.OK || result.ErrorType != "not_unique" || result.Metadata["match_count"] != 0 {
		t.Fatalf("zero match result = %+v", result)
	}
	data, _ = os.ReadFile(path)
	if string(data) != before {
		t.Fatal("file changed on zero match")
	}

	if err := os.WriteFile(path, []byte("x x x"), 0o644); err != nil {
		t.Fatal(err)
	}
	result = tool.Execute(context.Background(), json.RawMessage(`{"path":"file.txt","old_text":"x","new_text":"y"}`), DefaultEnv(dir))
	if result.OK || result.ErrorType != "not_unique" || result.Metadata["match_count"] != 3 {
		t.Fatalf("multi match result = %+v", result)
	}
	data, _ = os.ReadFile(path)
	if strings.TrimSpace(string(data)) != "x x x" {
		t.Fatal("file changed on multi match")
	}
}
