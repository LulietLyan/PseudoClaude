package permission

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"PseudoClaude/internal/llm"
	"PseudoClaude/internal/tools"
)

func TestPersist(t *testing.T) {
	root := t.TempDir()
	local := filepath.Join(root, ".PseudoClaude", "permissions.local.yaml")
	engine, err := NewEngine(root, Options{LocalPath: local})
	if err != nil {
		t.Fatal(err)
	}
	call := llm.ToolCall{Name: "write_file", Arguments: json.RawMessage(`{"path":"src/a.txt","content":"x"}`)}
	if got := engine.Check(ModeDefault, call, tools.SafetySideEffect); got.Decision != DecisionAsk {
		t.Fatalf("before session allow = %+v", got)
	}
	if err := engine.AllowForSession(call); err != nil {
		t.Fatal(err)
	}
	if got := engine.Check(ModeDefault, call, tools.SafetySideEffect); got.Decision != DecisionAllow || got.Source != "rule" {
		t.Fatalf("after session allow = %+v", got)
	}
	if _, err := os.Stat(local); !os.IsNotExist(err) {
		t.Fatalf("session allow should not write local file: %v", err)
	}

	cmd := llm.ToolCall{Name: "run_command", Arguments: json.RawMessage(`{"command":"git","args":["status"]}`)}
	if err := engine.PersistLocalAllow(cmd); err != nil {
		t.Fatal(err)
	}
	if err := engine.PersistLocalAllow(cmd); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(local)
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(string(data), "Bash(git status)"); count != 1 {
		t.Fatalf("local rules not deduped: %s", data)
	}
	reloaded, err := NewEngine(root, Options{LocalPath: local})
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Check(ModeDefault, cmd, tools.SafetySideEffect); got.Decision != DecisionAllow {
		t.Fatalf("persisted allow not effective: %+v", got)
	}
}
