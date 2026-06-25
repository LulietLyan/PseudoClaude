package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"PseudoClaude/internal/agent"
	"PseudoClaude/internal/conversation"
	"PseudoClaude/internal/task"
	"PseudoClaude/internal/team"
	base "PseudoClaude/internal/tools"
)

func TestCompatTaskToolsPreserveLegacyAndTeam(t *testing.T) {
	taskMgr := task.NewManager(task.Options{Run: func(ctx context.Context, runner agent.Runner, conv *conversation.Conversation, prompt string) agent.CompletionResult {
		return agent.CompletionResult{Text: "legacy done", Stop: agent.Stop{Reason: agent.StopCompleted}}
	}})
	legacyID, _ := taskMgr.Launch(context.Background(), task.LaunchInput{Name: "legacy", Prompt: "one"})
	waitTaskSnapshot(t, taskMgr, legacyID)

	teamMgr, err := team.NewManager(team.ManagerOptions{HomeDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := teamMgr.Create(context.Background(), team.CreateInput{Name: "demo"}); err != nil {
		t.Fatal(err)
	}
	create := NewTaskCreateTool(teamMgr)
	created := create.Execute(context.Background(), json.RawMessage(`{"team_name":"demo","title":"team task"}`), base.Env{})
	if !created.OK {
		t.Fatalf("team task create = %#v", created)
	}

	list := NewCompatTaskListTool(teamMgr, task.NewTaskListTool(taskMgr))
	legacyList := list.Execute(context.Background(), json.RawMessage(`{}`), base.Env{})
	if !legacyList.OK || !strings.Contains(legacyList.Content, legacyID) {
		t.Fatalf("legacy list = %#v", legacyList)
	}
	teamList := list.Execute(context.Background(), json.RawMessage(`{"team_name":"demo"}`), base.Env{})
	if !teamList.OK || !strings.Contains(teamList.Content, "team task") {
		t.Fatalf("team list = %#v", teamList)
	}

	get := NewCompatTaskGetTool(teamMgr, task.NewTaskGetTool(taskMgr))
	legacyGet := get.Execute(context.Background(), json.RawMessage(`{"task_id":"`+legacyID+`"}`), base.Env{})
	if !legacyGet.OK || !strings.Contains(legacyGet.Content, "legacy done") {
		t.Fatalf("legacy get = %#v", legacyGet)
	}
}

func TestCompatSendMessagePreservesLegacyAndTeam(t *testing.T) {
	taskMgr := task.NewManager(task.Options{Run: func(ctx context.Context, runner agent.Runner, conv *conversation.Conversation, prompt string) agent.CompletionResult {
		return agent.CompletionResult{Text: prompt, Stop: agent.Stop{Reason: agent.StopCompleted}}
	}})
	id, _ := taskMgr.Launch(context.Background(), task.LaunchInput{Name: "legacy", Prompt: "one"})
	waitTaskSnapshot(t, taskMgr, id)

	teamMgr, err := team.NewManager(team.ManagerOptions{HomeDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	created, err := teamMgr.Create(context.Background(), team.CreateInput{Name: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	if err := created.AddMember(teamMgr.HomeDir(), team.MemberInfo{Name: "alice", AgentID: "agent-a", BackendType: team.BackendInProcess}); err != nil {
		t.Fatal(err)
	}
	send := NewCompatSendMessageTool(teamMgr, task.NewSendMessageTool(taskMgr))
	legacy := send.Execute(context.Background(), json.RawMessage(`{"name":"legacy","message":"two"}`), base.Env{})
	if !legacy.OK || !strings.Contains(legacy.Content, "running") {
		t.Fatalf("legacy send = %#v", legacy)
	}
	teamResult := send.Execute(context.Background(), json.RawMessage(`{"team_name":"demo","to":"alice","summary":"hello"}`), base.Env{})
	if !teamResult.OK {
		t.Fatalf("team send = %#v", teamResult)
	}
	store, _ := teamMgr.MailboxStore("demo")
	msgs, _ := store.Read("agent-a")
	if len(msgs) != 1 || msgs[0].Summary != "hello" {
		t.Fatalf("team messages = %#v", msgs)
	}
	legacyNamedTeam := send.Execute(context.Background(), json.RawMessage(`{"name":"alice","message":"please report"}`), base.Env{})
	if !legacyNamedTeam.OK || strings.Contains(legacyNamedTeam.Content, "task_id") {
		t.Fatalf("legacy-shaped team send = %#v", legacyNamedTeam)
	}
	msgs, _ = store.Read("agent-a")
	if len(msgs) != 2 || msgs[1].Content != "please report" {
		t.Fatalf("legacy-shaped team messages = %#v", msgs)
	}
}

func waitTaskSnapshot(t *testing.T, manager *task.Manager, id string) {
	t.Helper()
	for event := range manager.SubscribeDone() {
		if event.TaskID == id {
			return
		}
	}
}
