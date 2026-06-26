package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"PseudoClaude/internal/agent"
	"PseudoClaude/internal/config"
	"PseudoClaude/internal/conversation"
	"PseudoClaude/internal/llm"
	"PseudoClaude/internal/subagent"
	"PseudoClaude/internal/task"
	"PseudoClaude/internal/team"
	"PseudoClaude/internal/team/mailbox"
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

func TestPendingRemindersIncludeLeadMailOnSubmit(t *testing.T) {
	home := t.TempDir()
	teamMgr, err := team.NewManager(team.ManagerOptions{HomeDir: home})
	if err != nil {
		t.Fatal(err)
	}
	created, err := teamMgr.Create(context.Background(), team.CreateInput{Name: "Demo", LeadAgentID: "lead-demo"})
	if err != nil {
		t.Fatal(err)
	}
	store, err := mailbox.New(created.InboxDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Write(context.Background(), "lead-demo", mailbox.Message{From: "agent-a", To: "lead-demo", Summary: "done", Content: "read result"}); err != nil {
		t.Fatal(err)
	}
	provider := &captureReminderProvider{seen: make(chan struct{})}
	model := New([]config.ProviderConfig{}, home, nil).WithTeams(teamMgr)
	model.provider = provider
	next, cmd := model.submitAgentTextWithTools("continue", "", nil)
	model = next.(Model)
	if model.state != stateStreaming || cmd == nil {
		t.Fatalf("state=%v cmd=%v", model.state, cmd)
	}
	runCmd(cmd)
	waitCaptureReminder(t, provider)
	if !strings.Contains(provider.reminder, "<team-update>") || !strings.Contains(provider.reminder, "read result") {
		t.Fatalf("reminder = %q", provider.reminder)
	}
	unread, err := store.ReadUnread("lead-demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(unread) != 0 {
		t.Fatalf("unread after submit = %#v", unread)
	}
}

func TestTeamWakeStartsPresetWhenLeadMailExists(t *testing.T) {
	home := t.TempDir()
	teamMgr, err := team.NewManager(team.ManagerOptions{HomeDir: home})
	if err != nil {
		t.Fatal(err)
	}
	created, err := teamMgr.Create(context.Background(), team.CreateInput{Name: "Demo", LeadAgentID: "lead-demo"})
	if err != nil {
		t.Fatal(err)
	}
	store, err := mailbox.New(created.InboxDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Write(context.Background(), "lead-demo", mailbox.Message{From: "agent-a", To: "lead-demo", Summary: "done"}); err != nil {
		t.Fatal(err)
	}
	provider := &captureReminderProvider{seen: make(chan struct{})}
	model := New([]config.ProviderConfig{}, home, nil).WithTeams(teamMgr)
	model.provider = provider
	next, cmd := model.Update(teamWakeMsg{})
	model = next.(Model)
	if model.state != stateStreaming || cmd == nil {
		t.Fatalf("state=%v cmd=%v", model.state, cmd)
	}
	runCmd(cmd)
	waitCaptureReminder(t, provider)
	if !strings.Contains(provider.reminder, "<team-update>") {
		t.Fatalf("reminder = %q", provider.reminder)
	}
}

func waitCaptureReminder(t *testing.T, provider *captureReminderProvider) {
	t.Helper()
	select {
	case <-provider.seen:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for provider request")
	}
}

type captureReminderProvider struct {
	reminder string
	seen     chan struct{}
}

func (p *captureReminderProvider) Name() string  { return "capture" }
func (p *captureReminderProvider) Model() string { return "capture-model" }
func (p *captureReminderProvider) Stream(ctx context.Context, req llm.Request) <-chan llm.StreamEvent {
	p.reminder = req.Reminder
	if p.seen != nil {
		close(p.seen)
	}
	ch := make(chan llm.StreamEvent, 1)
	ch <- llm.StreamEvent{Done: true}
	close(ch)
	return ch
}
