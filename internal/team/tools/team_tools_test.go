package tools

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"PseudoClaude/internal/team"
	base "PseudoClaude/internal/tools"
)

func TestTeamToolsCreateDeleteKill(t *testing.T) {
	mgr, err := team.NewManager(team.ManagerOptions{HomeDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	create := NewTeamCreateTool(mgr)
	result := create.Execute(context.Background(), json.RawMessage(`{"name":"Demo Team"}`), base.Env{})
	if !result.OK {
		t.Fatalf("create result = %#v", result)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(payload["config_path"].(string)); err != nil {
		t.Fatalf("config missing: %v", err)
	}
	created, ok := mgr.Get("Demo Team")
	if !ok || created.LeadAgentID != "lead-demo-team" {
		t.Fatalf("lead id = %q ok=%v", created.LeadAgentID, ok)
	}
	deleteTool := NewTeamDeleteTool(mgr)
	result = deleteTool.Execute(context.Background(), json.RawMessage(`{"team_name":"Demo Team","force":true}`), base.Env{})
	if !result.OK {
		t.Fatalf("delete result = %#v", result)
	}
}

func TestTaskToolsUseTeamContext(t *testing.T) {
	mgr, err := team.NewManager(team.ManagerOptions{HomeDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Create(context.Background(), team.CreateInput{Name: "demo"}); err != nil {
		t.Fatal(err)
	}
	env := base.Env{Team: &base.TeamEnv{TeamName: "demo", MemberName: "alice", AgentID: "agent-a"}}
	create := NewTaskCreateTool(mgr)
	result := create.Execute(context.Background(), json.RawMessage(`{"title":"Do work","assignee":"alice"}`), env)
	if !result.OK {
		t.Fatalf("task create = %#v", result)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(result.Content), &created); err != nil {
		t.Fatal(err)
	}
	list := NewTaskListTool(mgr)
	result = list.Execute(context.Background(), json.RawMessage(`{}`), env)
	if !result.OK || !contains(result.Content, created.ID) {
		t.Fatalf("task list = %#v", result)
	}
	update := NewTaskUpdateTool(mgr)
	result = update.Execute(context.Background(), json.RawMessage(`{"task_id":"`+created.ID+`","status":"done"}`), env)
	if !result.OK || !contains(result.Content, "done") {
		t.Fatalf("task update = %#v", result)
	}
}

func TestSendMessageWritesMailboxAndBroadcast(t *testing.T) {
	mgr, err := team.NewManager(team.ManagerOptions{HomeDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	created, err := mgr.Create(context.Background(), team.CreateInput{Name: "demo", LeadAgentID: "lead-a"})
	if err != nil {
		t.Fatal(err)
	}
	if err := created.AddMember(mgr.HomeDir(), team.MemberInfo{Name: "alice", AgentID: "agent-a", BackendType: team.BackendInProcess}); err != nil {
		t.Fatal(err)
	}
	if err := created.AddMember(mgr.HomeDir(), team.MemberInfo{Name: "bob", AgentID: "agent-b", BackendType: team.BackendInProcess}); err != nil {
		t.Fatal(err)
	}
	env := base.Env{Team: &base.TeamEnv{TeamName: "demo", MemberName: "lead", AgentID: "lead-a", IsLead: true}}
	send := NewSendMessageTool(mgr)
	result := send.Execute(context.Background(), json.RawMessage(`{"to":"alice","summary":"hello","content":"body"}`), env)
	if !result.OK {
		t.Fatalf("send = %#v", result)
	}
	store, err := mgr.MailboxStore("demo")
	if err != nil {
		t.Fatal(err)
	}
	msgs, err := store.Read("agent-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || msgs[0].Read || msgs[0].Summary != "hello" {
		t.Fatalf("messages = %#v", msgs)
	}
	result = send.Execute(context.Background(), json.RawMessage(`{"to":"broadcast","summary":"all"}`), env)
	if !result.OK || !contains(result.Content, "alice") || !contains(result.Content, "bob") {
		t.Fatalf("broadcast = %#v", result)
	}
}

func TestSendMessagePlanApprovalRequiresLead(t *testing.T) {
	mgr, err := team.NewManager(team.ManagerOptions{HomeDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	created, err := mgr.Create(context.Background(), team.CreateInput{Name: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	if err := created.AddMember(mgr.HomeDir(), team.MemberInfo{Name: "alice", AgentID: "agent-a", BackendType: team.BackendInProcess}); err != nil {
		t.Fatal(err)
	}
	send := NewSendMessageTool(mgr)
	env := base.Env{Team: &base.TeamEnv{TeamName: "demo", MemberName: "alice", AgentID: "agent-a"}}
	result := send.Execute(context.Background(), json.RawMessage(`{"to":"alice","type":"plan_approval_response","summary":"approved"}`), env)
	if result.OK || result.ErrorType != "forbidden" {
		t.Fatalf("result = %#v", result)
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
