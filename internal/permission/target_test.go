package permission

import (
	"encoding/json"
	"testing"

	"PseudoClaude/internal/llm"
	"PseudoClaude/internal/tools"
)

func TestTarget(t *testing.T) {
	for internal, friendly := range friendlyByInternal {
		if got := friendlyName(internal); got != friendly {
			t.Fatalf("friendlyName(%s) = %s, want %s", internal, got, friendly)
		}
	}
	if cat, ok := classify(llm.ToolCall{Name: "read_file"}, ""); !ok || cat != CategoryRead {
		t.Fatalf("read category = %s %v", cat, ok)
	}
	if cat, ok := classify(llm.ToolCall{Name: "run_command"}, ""); !ok || cat != CategoryExec {
		t.Fatalf("exec category = %s %v", cat, ok)
	}
	if cat, ok := classify(llm.ToolCall{Name: "custom"}, tools.SafetyReadOnly); !ok || cat != CategoryRead {
		t.Fatalf("custom readonly category = %s %v", cat, ok)
	}
	if cat, ok := classify(llm.ToolCall{Name: "mcp__github__get_issue"}, tools.SafetyReadOnly); !ok || cat != CategoryRead {
		t.Fatalf("mcp readonly category = %s %v", cat, ok)
	}
	if cat, ok := classify(llm.ToolCall{Name: "mcp__github__create_issue"}, tools.SafetySideEffect); !ok || cat != CategoryWrite {
		t.Fatalf("mcp side-effect category = %s %v", cat, ok)
	}
	if got := friendlyName("mcp__github__get_issue"); got != "mcp__github__get_issue" {
		t.Fatalf("mcp friendly = %q", got)
	}
	if got := internalName("mcp__github__*"); got != "mcp__github__*" {
		t.Fatalf("mcp internal = %q", got)
	}

	cmd, ok := commandText(llm.ToolCall{Name: "run_command", Arguments: json.RawMessage(`{"command":"git","args":["status","a b"]}`)})
	if !ok || cmd != `git status "a b"` {
		t.Fatalf("command = %q ok=%v", cmd, ok)
	}
	if _, ok := commandText(llm.ToolCall{Name: "run_command", Arguments: json.RawMessage(`{}`)}); ok {
		t.Fatal("missing command should fail")
	}
	target, match, ok := pathTarget(llm.ToolCall{Name: "search_code", Arguments: json.RawMessage(`{"pattern":"TODO"}`)})
	if !ok || target != "." || match != "." {
		t.Fatalf("search default path target=%q match=%q ok=%v", target, match, ok)
	}
	target, match, ok = pathTarget(llm.ToolCall{Name: "find_files", Arguments: json.RawMessage(`{"pattern":"src/**/*.go"}`)})
	if !ok || target != "src" || match != "src/**/*.go" {
		t.Fatalf("find target=%q match=%q ok=%v", target, match, ok)
	}
	if _, _, ok := pathTarget(llm.ToolCall{Name: "read_file", Arguments: json.RawMessage(`{}`)}); ok {
		t.Fatal("missing path should fail")
	}
	if !pathContainsGlob("*.env*") || !pathContainsGlob("src/[ab].go") || pathContainsGlob("README.md") {
		t.Fatal("glob path detection failed")
	}
	if !pathToolRequiresExactPath("read_file") || pathToolRequiresExactPath("find_files") {
		t.Fatal("exact path tool classification failed")
	}
}
