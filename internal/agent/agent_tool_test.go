package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"PseudoClaude/internal/conversation"
	"PseudoClaude/internal/llm"
	"PseudoClaude/internal/permission"
	"PseudoClaude/internal/subagent"
	"PseudoClaude/internal/tools"
)

func TestAgentToolDefinitionSchemaStable(t *testing.T) {
	project := t.TempDir()
	catalog := subagent.LoadCatalog(subagent.LoadOptions{ProjectRoot: project})
	tool := &AgentTool{Catalog: catalog}
	before, _ := json.Marshal(tool.Definition().InputSchema)
	writeProjectAgent(t, project, "demo")
	catalog.Reload(subagent.LoadOptions{ProjectRoot: project})
	after, _ := json.Marshal(tool.Definition().InputSchema)
	if string(before) != string(after) {
		t.Fatalf("schema changed:\n%s\n%s", before, after)
	}
	if !strings.Contains(tool.Definition().Description, "demo") {
		t.Fatalf("description did not include role summary: %q", tool.Definition().Description)
	}
	if tool.Definition().Timeout < time.Minute {
		t.Fatalf("Agent tool timeout too short: %s", tool.Definition().Timeout)
	}
}

func TestAgentToolUnknownSubagent(t *testing.T) {
	tool, _, _ := newAgentToolTestHarness(t, &fakeProvider{})
	result := tool.Execute(context.Background(), json.RawMessage(`{"prompt":"p","description":"d","subagent_type":"missing"}`), tools.Env{})
	if result.OK || result.ErrorType != "unknown_subagent_type" {
		t.Fatalf("result = %#v", result)
	}
}

func TestAgentToolDefinedSubagentForeground(t *testing.T) {
	provider := &fakeProvider{streams: [][]llm.StreamEvent{{{Text: "child done"}, {Done: true}}}}
	tool, _, parentConv := newAgentToolTestHarness(t, provider)
	parentConv.AddUser("parent")
	result := tool.Execute(context.Background(), json.RawMessage(`{"prompt":"child task","description":"d","subagent_type":"explore"}`), tools.Env{})
	if !result.OK || result.Content != "child done" {
		t.Fatalf("result = %#v", result)
	}
	if len(provider.requests) != 1 {
		t.Fatalf("provider requests = %d", len(provider.requests))
	}
	if len(provider.requests[0].Messages) != 1 || provider.requests[0].Messages[0].Content != "child task" {
		t.Fatalf("defined subagent should start blank: %#v", provider.requests[0].Messages)
	}
	for _, def := range provider.requests[0].Tools {
		if def.Name == "Agent" || def.Name == "write_file" {
			t.Fatalf("filtered tool leaked: %#v", provider.requests[0].Tools)
		}
	}
}

func TestAgentToolExploreAllowsReadOnlyTools(t *testing.T) {
	provider := &fakeProvider{streams: [][]llm.StreamEvent{
		{{ToolCall: &llm.ToolCall{ID: "read", Name: "read_file", Arguments: json.RawMessage(`{}`)}}, {Done: true}},
		{{Text: "read done"}, {Done: true}},
	}}
	tool, _, _ := newAgentToolTestHarness(t, provider)
	result := tool.Execute(context.Background(), json.RawMessage(`{"prompt":"read","description":"d","subagent_type":"explore"}`), tools.Env{})
	if !result.OK || !strings.Contains(result.Content, "read done") {
		t.Fatalf("result = %#v", result)
	}
	if len(provider.requests) != 2 {
		t.Fatalf("provider requests = %d, want read tool round + final", len(provider.requests))
	}
}

func TestAgentToolSubagentRunsWriteAfterAgentApproval(t *testing.T) {
	provider := &fakeProvider{streams: [][]llm.StreamEvent{
		{{ToolCall: &llm.ToolCall{ID: "write", Name: "write_file", Arguments: json.RawMessage(`{"path":"a","content":"b"}`)}}, {Done: true}},
		{{Text: "write done"}, {Done: true}},
	}}
	tool, _, _ := newAgentToolTestHarness(t, provider)
	snap := tool.Parent.Snapshot()
	snap.Approval = func(ctx context.Context, req ApprovalRequest) (permission.ApprovalDecision, error) {
		t.Fatal("subagent should not ask again after Agent tool approval")
		return permission.ApprovalDenyOnce, nil
	}
	tool.Parent.Store(snap)
	result := tool.Execute(context.Background(), json.RawMessage(`{"prompt":"write","description":"d","subagent_type":"general-purpose"}`), tools.Env{})
	if !result.OK || !strings.Contains(result.Content, "write done") {
		t.Fatalf("result = %#v", result)
	}
}

func TestAgentToolSubagentStillRespectsDeny(t *testing.T) {
	provider := &fakeProvider{streams: [][]llm.StreamEvent{
		{{ToolCall: &llm.ToolCall{ID: "danger", Name: "run_command", Arguments: json.RawMessage(`{"command":"rm","args":["-rf","/"]}`)}}, {Done: true}},
		{{Text: "safe followup"}, {Done: true}},
	}}
	tool, _, _ := newAgentToolTestHarness(t, provider)
	result := tool.Execute(context.Background(), json.RawMessage(`{"prompt":"danger","description":"d","subagent_type":"general-purpose"}`), tools.Env{})
	if !result.OK || !strings.Contains(result.Content, "safe followup") {
		t.Fatalf("result = %#v", result)
	}
}

func TestAgentToolDefinedSubagentBackground(t *testing.T) {
	provider := &fakeProvider{streams: [][]llm.StreamEvent{{{Text: "done"}, {Done: true}}}}
	tool, launcher, _ := newAgentToolTestHarness(t, provider)
	result := tool.Execute(context.Background(), json.RawMessage(`{"prompt":"child task","description":"d","subagent_type":"explore","run_in_background":true,"name":"bg"}`), tools.Env{})
	if !result.OK || !strings.Contains(result.Content, "async_launched") {
		t.Fatalf("result = %#v", result)
	}
	if len(launcher.inputs) != 1 || launcher.inputs[0].Name != "bg" || launcher.inputs[0].Type != "explore" {
		t.Fatalf("launches = %#v", launcher.inputs)
	}
}

func TestAgentToolBackgroundDoesNotCaptureApprovalUpgrader(t *testing.T) {
	provider := &fakeProvider{streams: [][]llm.StreamEvent{{{Text: "done"}, {Done: true}}}}
	tool, launcher, _ := newAgentToolTestHarness(t, provider)
	snap := tool.Parent.Snapshot()
	snap.Approval = func(ctx context.Context, req ApprovalRequest) (permission.ApprovalDecision, error) {
		t.Fatal("background task should not capture foreground approval upgrader")
		return permission.ApprovalAllowOnce, nil
	}
	tool.Parent.Store(snap)
	result := tool.Execute(context.Background(), json.RawMessage(`{"prompt":"child task","description":"d","subagent_type":"general-purpose","run_in_background":true}`), tools.Env{})
	if !result.OK || len(launcher.inputs) != 1 {
		t.Fatalf("result = %#v launches=%#v", result, launcher.inputs)
	}
	if launcher.inputs[0].Runner.Sub.ApprovalUpgrader == nil || !launcher.inputs[0].Runner.Sub.DontAsk {
		t.Fatalf("background runner should have non-blocking default authorization: %#v", launcher.inputs[0].Runner.Sub)
	}
}

func TestAgentToolForkSubagent(t *testing.T) {
	provider := &fakeProvider{streams: [][]llm.StreamEvent{{{Text: "fork done"}, {Done: true}}}}
	tool, launcher, parentConv := newAgentToolTestHarness(t, provider)
	parentConv.AddUser("parent")
	result := tool.Execute(context.Background(), json.RawMessage(`{"prompt":"fork task","description":"d","run_in_background":false}`), tools.Env{})
	if !result.OK || !strings.Contains(result.Content, "async_launched") {
		t.Fatalf("result = %#v", result)
	}
	if len(launcher.inputs) != 1 || !launcher.inputs[0].Fork {
		t.Fatalf("launches = %#v", launcher.inputs)
	}
	if !subagent.IsForkMessages(launcher.inputs[0].Conversation.Messages()) {
		t.Fatalf("fork marker missing: %#v", launcher.inputs[0].Conversation.Messages())
	}
}

func TestAgentToolTimeoutToBackground(t *testing.T) {
	blocking := blockingProvider{}
	tool, launcher, _ := newAgentToolTestHarness(t, &blocking)
	tool.Background.ForegroundTimeout = 10 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := tool.Execute(ctx, json.RawMessage(`{"prompt":"wait","description":"d","subagent_type":"general-purpose"}`), tools.Env{})
	if !result.OK || !strings.Contains(result.Content, "timed_out_to_background") {
		t.Fatalf("result = %#v", result)
	}
	if len(launcher.inputs) != 1 {
		t.Fatalf("launches = %#v", launcher.inputs)
	}
}

func TestAgentToolSubagentPermissionDoesNotHangWithoutUpgrader(t *testing.T) {
	provider := &fakeProvider{streams: [][]llm.StreamEvent{
		{{ToolCall: &llm.ToolCall{ID: "write", Name: "write_file", Arguments: json.RawMessage(`{"path":"a","content":"b"}`)}}, {Done: true}},
		{{Text: "done"}, {Done: true}},
	}}
	tool, _, _ := newAgentToolTestHarness(t, provider)
	result := tool.Execute(context.Background(), json.RawMessage(`{"prompt":"child task","description":"d","subagent_type":"general-purpose"}`), tools.Env{})
	if !result.OK || !strings.Contains(result.Content, "done") {
		t.Fatalf("result = %#v", result)
	}
}

func newAgentToolTestHarness(t *testing.T, provider llm.Provider) (*AgentTool, *testLauncher, *conversation.Conversation) {
	t.Helper()
	catalog := subagent.LoadCatalog(subagent.LoadOptions{})
	registry, err := tools.NewRegistry(
		fakeExecTool{name: "Agent", safety: tools.SafetySideEffect},
		fakeExecTool{name: "read_file", safety: tools.SafetyReadOnly},
		fakeExecTool{name: "write_file", safety: tools.SafetySideEffect},
		fakeExecTool{name: "edit_file", safety: tools.SafetySideEffect},
	)
	if err != nil {
		t.Fatal(err)
	}
	parentConv := &conversation.Conversation{}
	handle := &RunnerHandle{}
	handle.Store(RunnerSnapshot{
		Provider:     provider,
		Registry:     registry,
		Conversation: parentConv,
		Config:       DefaultConfig(),
		Env:          tools.Env{},
		Permission:   newTestPermissionEngine(t),
	})
	launcher := &testLauncher{}
	return &AgentTool{Catalog: catalog, Tasks: launcher, Parent: handle}, launcher, parentConv
}

type testLauncher struct {
	inputs []AgentLaunchInput
}

func (l *testLauncher) LaunchAgent(ctx context.Context, in AgentLaunchInput) (string, error) {
	l.inputs = append(l.inputs, in)
	return "task-agent-test-" + string(rune('0'+len(l.inputs))), nil
}

func writeProjectAgent(t *testing.T, project, name string) {
	t.Helper()
	path := project + "/.PseudoClaude/agents/" + name + ".md"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := `---
name: ` + name + `
description: Demo.
---
Prompt.
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

type blockingProvider struct{}

func (blockingProvider) Name() string  { return "blocking" }
func (blockingProvider) Model() string { return "blocking" }
func (blockingProvider) Stream(ctx context.Context, req llm.Request) <-chan llm.StreamEvent {
	ch := make(chan llm.StreamEvent)
	go func() {
		defer close(ch)
		<-ctx.Done()
	}()
	return ch
}
