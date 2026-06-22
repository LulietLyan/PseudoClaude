package subagent

import (
	"encoding/json"
	"strings"
	"testing"

	"PseudoClaude/internal/llm"
)

func TestForkMessagesDeepCopyAndMarker(t *testing.T) {
	parent := []llm.Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "call-1", Name: "read_file", Arguments: json.RawMessage(`{"path":"a"}`)}}},
	}
	child := BuildForkMessages(parent, "inspect this")
	if len(child) != 4 {
		t.Fatalf("child len = %d, want 4: %#v", len(child), child)
	}
	if child[0].Content != parent[0].Content || child[1].ToolCalls[0].ID != parent[1].ToolCalls[0].ID {
		t.Fatalf("prefix not preserved: %#v", child)
	}
	child[1].ToolCalls[0].ID = "changed"
	if parent[1].ToolCalls[0].ID == "changed" {
		t.Fatal("child mutation changed parent tool calls")
	}
	if child[2].ToolResult == nil || !child[2].ToolResult.IsError {
		t.Fatalf("dangling tool call not patched: %#v", child[2])
	}
	last := child[len(child)-1]
	if last.Role != "user" || !strings.Contains(last.Content, ForkBoilerplateTag) || !strings.Contains(last.Content, "inspect this") {
		t.Fatalf("last message missing fork task: %#v", last)
	}
	if !IsForkMessages(child) {
		t.Fatal("IsForkMessages did not detect fork marker")
	}
}
