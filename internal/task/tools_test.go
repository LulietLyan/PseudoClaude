package task

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"PseudoClaude/internal/agent"
	"PseudoClaude/internal/conversation"
	"PseudoClaude/internal/tools"
)

func TestTaskTools(t *testing.T) {
	manager := NewManager(Options{IDSource: seqIDs(), Run: func(ctx context.Context, runner agent.Runner, conv *conversation.Conversation, prompt string) agent.CompletionResult {
		return agent.CompletionResult{Text: "done", Stop: agent.Stop{Reason: agent.StopCompleted}}
	}})
	id, _ := manager.Launch(context.Background(), LaunchInput{Name: "demo", Prompt: "one"})
	waitDone(t, manager, id)

	list := NewTaskListTool(manager).Execute(context.Background(), json.RawMessage(`{}`), tools.Env{})
	if !list.OK || !strings.Contains(list.Content, id) || !strings.Contains(list.Content, "completed") {
		t.Fatalf("TaskList = %#v", list)
	}
	get := NewTaskGetTool(manager).Execute(context.Background(), json.RawMessage(`{"task_id":"`+id+`"}`), tools.Env{})
	if !get.OK || !strings.Contains(get.Content, `"result":"done"`) {
		t.Fatalf("TaskGet = %#v", get)
	}
	missing := NewTaskGetTool(manager).Execute(context.Background(), json.RawMessage(`{"task_id":"nope"}`), tools.Env{})
	if missing.OK || missing.ErrorType != "not_found" {
		t.Fatalf("missing TaskGet = %#v", missing)
	}
	next := NewSendMessageTool(manager).Execute(context.Background(), json.RawMessage(`{"name":"demo","message":"two"}`), tools.Env{})
	if !next.OK || !strings.Contains(next.Content, "running") {
		t.Fatalf("SendMessage = %#v", next)
	}
	stop := NewTaskStopTool(manager).Execute(context.Background(), json.RawMessage(`{"task_id":"`+id+`"}`), tools.Env{})
	if !stop.OK || !strings.Contains(stop.Content, "cancellation_requested") {
		t.Fatalf("TaskStop = %#v", stop)
	}
	invalid := NewSendMessageTool(manager).Execute(context.Background(), json.RawMessage(`{"name":""}`), tools.Env{})
	if invalid.OK || invalid.ErrorType != "invalid_arguments" {
		t.Fatalf("invalid SendMessage = %#v", invalid)
	}
}

func TestTaskListSchemaRequiredIsArray(t *testing.T) {
	def := NewTaskListTool(NewManager(Options{})).Definition()
	data, err := json.Marshal(def.InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"required":null`) {
		t.Fatalf("required must not be null: %s", data)
	}
	if !strings.Contains(string(data), `"required":[]`) {
		t.Fatalf("required should be an empty array: %s", data)
	}
}
