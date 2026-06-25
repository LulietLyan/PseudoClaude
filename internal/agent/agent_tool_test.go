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
	"PseudoClaude/internal/worktree"
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
	schema := string(after)
	if !strings.Contains(schema, "team_name") || !strings.Contains(schema, "plan_mode_required") {
		t.Fatalf("team launch fields missing from schema: %s", schema)
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

func TestAgentToolWorktreeUnavailable(t *testing.T) {
	project := t.TempDir()
	writeProjectAgentWithIsolation(t, project, "iso")
	tool, _, _ := newAgentToolTestHarness(t, &fakeProvider{})
	tool.Catalog = subagent.LoadCatalog(subagent.LoadOptions{ProjectRoot: project})
	result := tool.Execute(context.Background(), json.RawMessage(`{"prompt":"child task","description":"d","subagent_type":"iso"}`), tools.Env{})
	if result.OK || result.ErrorType != "worktree_unavailable" {
		t.Fatalf("result = %#v", result)
	}
}

func TestAgentToolWorktreeBackgroundPrepare(t *testing.T) {
	project := t.TempDir()
	writeProjectAgentWithIsolation(t, project, "iso")
	tool, launcher, _ := newAgentToolTestHarness(t, &fakeProvider{})
	tool.Catalog = subagent.LoadCatalog(subagent.LoadOptions{ProjectRoot: project})
	tool.Worktrees = &fakeWorktreeService{wt: &worktree.Worktree{Name: "agent-a1234567", Path: "/tmp/wt", Branch: "worktree-agent-a1234567"}}
	result := tool.Execute(context.Background(), json.RawMessage(`{"prompt":"child task","description":"d","subagent_type":"iso","run_in_background":true}`), tools.Env{})
	if !result.OK || len(launcher.inputs) != 1 {
		t.Fatalf("result = %#v inputs=%#v", result, launcher.inputs)
	}
	if launcher.inputs[0].Prepare == nil {
		t.Fatal("background worktree launch did not include prepare")
	}
}

func TestAgentToolPerCallWorktreeIsolation(t *testing.T) {
	tool, launcher, _ := newAgentToolTestHarness(t, &fakeProvider{})
	tool.Worktrees = &fakeWorktreeService{wt: &worktree.Worktree{Name: "agent-a1234567", Path: "/tmp/wt", Branch: "worktree-agent-a1234567"}}
	result := tool.Execute(context.Background(), json.RawMessage(`{"prompt":"child task","description":"d","subagent_type":"general-purpose","isolation":"worktree","run_in_background":true}`), tools.Env{})
	if !result.OK || len(launcher.inputs) != 1 {
		t.Fatalf("result = %#v inputs=%#v", result, launcher.inputs)
	}
	if launcher.inputs[0].Type != "general-purpose" || launcher.inputs[0].Prepare == nil {
		t.Fatalf("per-call worktree isolation did not prepare worktree launch: %#v", launcher.inputs[0])
	}
}

func TestAgentToolRejectsUnknownIsolation(t *testing.T) {
	tool, _, _ := newAgentToolTestHarness(t, &fakeProvider{})
	result := tool.Execute(context.Background(), json.RawMessage(`{"prompt":"child task","description":"d","subagent_type":"general-purpose","isolation":"container"}`), tools.Env{})
	if result.OK || result.ErrorType != "invalid_arguments" {
		t.Fatalf("result = %#v", result)
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

func TestAgentToolDelegatesTeamLaunch(t *testing.T) {
	tool, _, _ := newAgentToolTestHarness(t, &fakeProvider{})
	service := &fakeTeamService{}
	tool.Team = service
	result := tool.Execute(context.Background(), json.RawMessage(`{"prompt":"p","description":"d","subagent_type":"general-purpose","name":"alice","team_name":"demo","plan_mode_required":true}`), tools.Env{})
	if !result.OK {
		t.Fatalf("result = %#v", result)
	}
	if len(service.inputs) != 1 {
		t.Fatalf("team launches = %#v", service.inputs)
	}
	in := service.inputs[0]
	if in.TeamName != "demo" || in.MemberName != "alice" || in.SubagentType != "general-purpose" || !in.PlanModeRequired {
		t.Fatalf("team launch input = %+v", in)
	}
	if !strings.Contains(result.Content, "agent-a") {
		t.Fatalf("result content = %q", result.Content)
	}
}

func TestAgentToolTeamLaunchDefaultsDescription(t *testing.T) {
	tool, _, _ := newAgentToolTestHarness(t, &fakeProvider{})
	service := &fakeTeamService{}
	tool.Team = service
	result := tool.Execute(context.Background(), json.RawMessage(`{"prompt":"read README.md and report the headings","team_name":"demo","name":"alice"}`), tools.Env{})
	if !result.OK {
		t.Fatalf("result = %#v", result)
	}
	if len(service.inputs) != 1 || service.inputs[0].Description == "" {
		t.Fatalf("team launch input = %#v", service.inputs)
	}
}

func TestAgentToolRejectsMissingPrompt(t *testing.T) {
	tool, _, _ := newAgentToolTestHarness(t, &fakeProvider{})
	result := tool.Execute(context.Background(), json.RawMessage(`{"description":"d","team_name":"demo","name":"alice"}`), tools.Env{})
	if result.OK || result.ErrorType != "invalid_arguments" || !strings.Contains(result.Error, "prompt") {
		t.Fatalf("result = %#v", result)
	}
}

func TestAgentToolRejectsTeamLaunchFromSubagent(t *testing.T) {
	tool, _, _ := newAgentToolTestHarness(t, &fakeProvider{})
	service := &fakeTeamService{}
	tool.Team = service
	snap := tool.Parent.Snapshot()
	snap.Sub.IsSubAgent = true
	tool.Parent.Store(snap)
	result := tool.Execute(context.Background(), json.RawMessage(`{"prompt":"p","description":"d","team_name":"demo","name":"alice"}`), tools.Env{})
	if result.OK || result.ErrorType != "nested_team_member_forbidden" {
		t.Fatalf("result = %#v", result)
	}
	if len(service.inputs) != 0 {
		t.Fatalf("unexpected team launches = %#v", service.inputs)
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

type fakeTeamService struct {
	inputs []TeamLaunchInput
}

func (s *fakeTeamService) SpawnMember(ctx context.Context, in TeamLaunchInput) (TeamLaunchResult, error) {
	s.inputs = append(s.inputs, in)
	return TeamLaunchResult{TeamName: in.TeamName, MemberName: in.MemberName, AgentID: "agent-a", Backend: "in-process", WorktreePath: "/tmp/wt", SessionDir: "/tmp/session"}, nil
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

func writeProjectAgentWithIsolation(t *testing.T, project, name string) {
	t.Helper()
	path := project + "/.PseudoClaude/agents/" + name + ".md"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := `---
name: ` + name + `
description: Demo.
isolation: worktree
---
Prompt.
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

type fakeWorktreeService struct {
	wt *worktree.Worktree
}

func (f *fakeWorktreeService) Create(ctx context.Context, in worktree.CreateInput) (*worktree.Worktree, error) {
	wt := *f.wt
	wt.Name = in.Name
	return &wt, nil
}

func (f *fakeWorktreeService) AutoCleanup(ctx context.Context, name string) (*worktree.AutoCleanupReport, error) {
	return &worktree.AutoCleanupReport{Name: name, Path: f.wt.Path, Branch: f.wt.Branch}, nil
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
