package permission

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"PseudoClaude/internal/llm"
	"PseudoClaude/internal/tools"
)

func TestEngine(t *testing.T) {
	root := t.TempDir()
	local := filepath.Join(root, ".PseudoClaude", "permissions.local.yaml")
	project := filepath.Join(root, ".PseudoClaude", "permissions.yaml")
	user := filepath.Join(t.TempDir(), "permissions.yaml")
	mustWrite(t, project, "permissions:\n  allow:\n    - Bash(git status)\n    - Read\n")
	mustWrite(t, local, "permissions:\n  deny:\n    - Bash(git push)\n")
	engine, err := NewEngine(root, Options{UserPath: user, ProjectPath: project, LocalPath: local})
	if err != nil {
		t.Fatal(err)
	}

	danger := llm.ToolCall{ID: "1", Name: "run_command", Arguments: json.RawMessage(`{"command":"rm","args":["-rf","/"]}`)}
	got := engine.Check(ModeBypassPermissions, danger, tools.SafetySideEffect)
	if got.Decision != DecisionDeny || got.Source != "blacklist" {
		t.Fatalf("blacklist result = %+v", got)
	}
	push := llm.ToolCall{ID: "2", Name: "run_command", Arguments: json.RawMessage(`{"command":"git","args":["push"]}`)}
	got = engine.Check(ModeBypassPermissions, push, tools.SafetySideEffect)
	if got.Decision != DecisionDeny || got.Source != "rule" {
		t.Fatalf("local deny result = %+v", got)
	}
	status := llm.ToolCall{ID: "3", Name: "run_command", Arguments: json.RawMessage(`{"command":"git","args":["status"]}`)}
	got = engine.Check(ModeDefault, status, tools.SafetySideEffect)
	if got.Decision != DecisionAllow || got.Source != "rule" {
		t.Fatalf("project allow result = %+v", got)
	}
	read := llm.ToolCall{ID: "4", Name: "read_file", Arguments: json.RawMessage(`{"path":"README.md"}`)}
	got = engine.Check(ModeDefault, read, tools.SafetyReadOnly)
	if got.Decision != DecisionAllow || got.Source != "rule" {
		t.Fatalf("read allow result = %+v", got)
	}
	outside := llm.ToolCall{ID: "5", Name: "read_file", Arguments: json.RawMessage(`{"path":"../outside"}`)}
	got = engine.Check(ModeBypassPermissions, outside, tools.SafetyReadOnly)
	if got.Decision != DecisionDeny || got.Source != "sandbox" {
		t.Fatalf("sandbox result = %+v", got)
	}
	write := llm.ToolCall{ID: "6", Name: "write_file", Arguments: json.RawMessage(`{"path":"new/file.txt","content":"x"}`)}
	got = engine.Check(ModeDefault, write, tools.SafetySideEffect)
	if got.Decision != DecisionAsk || got.Source != "mode" {
		t.Fatalf("write default result = %+v", got)
	}
	unknown := llm.ToolCall{ID: "7", Name: "missing", Arguments: json.RawMessage(`{}`)}
	got = engine.Check(ModeBypassPermissions, unknown, "")
	if got.Decision == DecisionAllow {
		t.Fatalf("unknown should not allow: %+v", got)
	}
	badArgs := llm.ToolCall{ID: "8", Name: "read_file", Arguments: json.RawMessage(`{`)}
	got = engine.Check(ModeBypassPermissions, badArgs, tools.SafetyReadOnly)
	if got.Decision != DecisionDeny {
		t.Fatalf("bad args should deny: %+v", got)
	}
	readGlob := llm.ToolCall{ID: "9", Name: "read_file", Arguments: json.RawMessage(`{"path":"*.env*"}`)}
	got = engine.Check(ModeBypassPermissions, readGlob, tools.SafetyReadOnly)
	if got.Decision != DecisionDeny || got.Source != "unknown" {
		t.Fatalf("read glob should deny before execution: %+v", got)
	}
	findGlob := llm.ToolCall{ID: "10", Name: "find_files", Arguments: json.RawMessage(`{"pattern":"*.env*"}`)}
	got = engine.Check(ModeDefault, findGlob, tools.SafetyReadOnly)
	if got.Decision != DecisionAllow {
		t.Fatalf("find_files glob should remain allowed: %+v", got)
	}
}

func TestEngineRulePrecedence(t *testing.T) {
	root := t.TempDir()
	user := filepath.Join(t.TempDir(), "user.yaml")
	project := filepath.Join(root, ".PseudoClaude", "permissions.yaml")
	local := filepath.Join(root, ".PseudoClaude", "permissions.local.yaml")
	mustWrite(t, user, "permissions:\n  allow:\n    - Bash(git push)\n")
	mustWrite(t, project, "permissions:\n  deny:\n    - Bash(git push)\n")
	mustWrite(t, local, "permissions:\n  allow:\n    - Bash(git push)\n")
	engine, err := NewEngine(root, Options{UserPath: user, ProjectPath: project, LocalPath: local})
	if err != nil {
		t.Fatal(err)
	}
	call := llm.ToolCall{Name: "run_command", Arguments: json.RawMessage(`{"command":"git","args":["push"]}`)}
	got := engine.Check(ModeDefault, call, tools.SafetySideEffect)
	if got.Decision != DecisionAllow {
		t.Fatalf("local should beat project/user: %+v", got)
	}
	if err := engine.AllowForSession(call); err != nil {
		t.Fatal(err)
	}
	deny, _ := parseRule("Bash(git push)", DecisionDeny)
	engine.session.Deny = append(engine.session.Deny, deny)
	got = engine.Check(ModeDefault, call, tools.SafetySideEffect)
	if got.Decision != DecisionDeny {
		t.Fatalf("session deny should beat all: %+v", got)
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
