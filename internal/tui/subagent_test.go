package tui

import (
	"context"
	"strings"
	"testing"

	"PseudoClaude/internal/agent"
	"PseudoClaude/internal/config"
	"PseudoClaude/internal/conversation"
	"PseudoClaude/internal/subagent"
	"PseudoClaude/internal/task"
)

func TestTaskNotificationIntegration(t *testing.T) {
	manager := task.NewManager(task.Options{
		IDSource: func() string { return "task-notify" },
		Run: func(ctx context.Context, runner agent.Runner, conv *conversation.Conversation, prompt string) agent.CompletionResult {
			return agent.CompletionResult{Text: "done", Stop: agent.Stop{Reason: agent.StopCompleted}}
		},
	})
	model := New([]config.ProviderConfig{}, t.TempDir(), nil).WithSubAgents(subagent.LoadCatalog(subagent.LoadOptions{}), manager)
	id, err := manager.Launch(context.Background(), task.LaunchInput{Name: "demo", Type: "explore", Prompt: "x"})
	if err != nil {
		t.Fatal(err)
	}
	next, cmd := model.Update(taskDoneMsg{id: id})
	model = next.(Model)
	if cmd == nil {
		t.Fatal("expected resubscribe command")
	}
	if len(model.pendingTaskNotes) != 1 || !strings.Contains(model.pendingTaskNotes[0], "<task-notification>") || !strings.Contains(model.pendingTaskNotes[0], "task-notify") {
		t.Fatalf("pending notes = %#v", model.pendingTaskNotes)
	}
	drained := model.drainTaskNotifications()
	if len(drained) != 1 || len(model.pendingTaskNotes) != 0 {
		t.Fatalf("drain = %#v pending=%#v", drained, model.pendingTaskNotes)
	}
}
