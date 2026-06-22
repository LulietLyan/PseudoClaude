package agent

import (
	"context"
	"encoding/json"
	"testing"

	"PseudoClaude/internal/llm"
	"PseudoClaude/internal/permission"
	"PseudoClaude/internal/tools"
)

func TestRunnerHandleSnapshotCopiesAllowedTools(t *testing.T) {
	handle := &RunnerHandle{}
	allowed := []string{"read_file"}
	handle.Store(RunnerSnapshot{AllowedTools: allowed})
	allowed[0] = "write_file"
	snap := handle.Snapshot()
	if len(snap.AllowedTools) != 1 || snap.AllowedTools[0] != "read_file" {
		t.Fatalf("snapshot did not copy allowed tools: %#v", snap.AllowedTools)
	}
	snap.AllowedTools[0] = "edit_file"
	again := handle.Snapshot()
	if again.AllowedTools[0] != "read_file" {
		t.Fatalf("snapshot mutation leaked into handle: %#v", again.AllowedTools)
	}
}

func TestSubRunApprovalSourceLabel(t *testing.T) {
	call := llm.ToolCall{ID: "write", Name: "write_file", Arguments: json.RawMessage(`{"path":"a","content":"b"}`)}
	registry, _ := tools.NewRegistry(fakeExecTool{name: "write_file", safety: tools.SafetySideEffect})
	engine := newTestPermissionEngine(t)
	var got ApprovalRequest
	opts := toolExecutionOptions{Sub: SubRunOptions{
		IsSubAgent:  true,
		ParentLabel: "explore/task-1",
		ApprovalUpgrader: func(ctx context.Context, req ApprovalRequest) (permission.ApprovalDecision, error) {
			got = req
			return permission.ApprovalDenyOnce, nil
		},
	}}
	events := make(chan Event, 8)
	result := permissionCheckedTool(context.Background(), registry, tools.Env{}, 1, call, events, opts, engine, permission.ModeDefault, toolHookContext{})
	if result.ErrorType != "permission_denied" {
		t.Fatalf("result = %#v", result)
	}
	if got.SourceLabel != "explore/task-1" {
		t.Fatalf("source label = %q", got.SourceLabel)
	}
}

func TestSubRunZeroValueLabel(t *testing.T) {
	if got := (SubRunOptions{}).label(); got != "" {
		t.Fatalf("zero label = %q", got)
	}
}
