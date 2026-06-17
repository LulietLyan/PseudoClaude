package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"PseudoClaude/internal/llm"
)

func TestContextWriterListAndLoad(t *testing.T) {
	workspace := t.TempDir()
	ctx, err := NewContext(workspace, time.Date(2026, 6, 17, 12, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ParseID(ctx.ID); !ok {
		t.Fatalf("invalid session id: %s", ctx.ID)
	}
	writer, err := NewWriter(ctx, "test-model", nil)
	if err != nil {
		t.Fatal(err)
	}
	writer.AppendMessage(llm.Message{Role: "user", Content: "hello"})
	writer.AppendMessage(llm.Message{Role: "assistant", Content: "hi"})
	writer.AppendReplace(ReplaceCompact, []llm.Message{{Role: "user", Content: "after compact"}})
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(ctx.JSONLPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var entry Entry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("bad json line %q: %v", line, err)
		}
	}
	infos, err := List(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 || infos[0].Title != "hello" || infos[0].Model != "test-model" {
		t.Fatalf("infos = %+v", infos)
	}
	loaded, err := Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Messages) != 1 || loaded.Messages[0].Content != "after compact" {
		t.Fatalf("loaded messages = %+v", loaded.Messages)
	}
}

func TestLoadSkipsBadLinesAndTruncatesDanglingToolCalls(t *testing.T) {
	workspace := t.TempDir()
	ctx, err := NewContext(workspace, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	lines := []string{
		`{"type":"message","role":"user","content":"hello","ts":1}`,
		`not-json`,
		`{"type":"message","role":"assistant","tool_calls":[{"id":"call_1","name":"read_file","arguments":{"path":"a"}}],"ts":2}`,
	}
	if err := os.WriteFile(ctx.JSONLPath, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.BadLines != 1 || !loaded.Truncated || len(loaded.Messages) != 1 {
		t.Fatalf("loaded = %+v", loaded)
	}
}

func TestCleanExpiredKeepsUnknownIDs(t *testing.T) {
	workspace := t.TempDir()
	oldID := "20260501-120000-abcd"
	newID := "20260616-120000-abcd"
	for _, id := range []string{oldID, newID, "legacy"} {
		if err := os.MkdirAll(filepath.Join(workspace, ".PseudoClaude", SessionsDirName, id), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	errs := CleanExpired(workspace, time.Date(2026, 6, 17, 12, 0, 0, 0, time.Local))
	if len(errs) != 0 {
		t.Fatalf("clean errors: %v", errs)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".PseudoClaude", SessionsDirName, oldID)); !os.IsNotExist(err) {
		t.Fatalf("old session still exists or stat failed: %v", err)
	}
	for _, id := range []string{newID, "legacy"} {
		if _, err := os.Stat(filepath.Join(workspace, ".PseudoClaude", SessionsDirName, id)); err != nil {
			t.Fatalf("%s should remain: %v", id, err)
		}
	}
}
