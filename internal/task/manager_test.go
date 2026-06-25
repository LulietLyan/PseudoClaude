package task

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"PseudoClaude/internal/agent"
	"PseudoClaude/internal/conversation"
)

func TestManagerLaunchCompleteAndSnapshot(t *testing.T) {
	manager := NewManager(Options{
		IDSource: seqIDs(),
		Run: func(ctx context.Context, runner agent.Runner, conv *conversation.Conversation, prompt string) agent.CompletionResult {
			conv.AddUser(prompt)
			return agent.CompletionResult{Text: "done " + prompt, Stop: agent.Stop{Reason: agent.StopCompleted}, ToolCount: 1, LastTool: "read_file"}
		},
	})
	id, err := manager.Launch(context.Background(), LaunchInput{Name: "demo", Type: "explore", Prompt: "task", Conversation: &conversation.Conversation{}})
	if err != nil {
		t.Fatal(err)
	}
	waitDone(t, manager, id)
	snap, ok := manager.Get(id)
	if !ok || snap.Status != StatusCompleted || snap.Result != "done task" || snap.ToolCount != 1 || !strings.Contains(snap.LastActivity, "read_file") {
		t.Fatalf("snapshot = %#v ok=%v", snap, ok)
	}
	snap.Result = "mutated"
	again, _ := manager.Get(id)
	if again.Result == "mutated" {
		t.Fatal("snapshot mutation affected manager")
	}
}

func TestManagerPreassignedIDOnFinishAndMultiSubscribe(t *testing.T) {
	var finished atomic.Int32
	manager := NewManager(Options{
		IDSource: seqIDs(),
		Run: func(ctx context.Context, runner agent.Runner, conv *conversation.Conversation, prompt string) agent.CompletionResult {
			return agent.CompletionResult{Text: "done", Stop: agent.Stop{Reason: agent.StopCompleted}}
		},
	})
	subA := manager.SubscribeDone()
	subB := manager.SubscribeDone()
	id, err := manager.Launch(context.Background(), LaunchInput{
		ID:     "agent-fixed",
		Prompt: "task",
		OnFinish: func(ctx context.Context, event FinishEvent) {
			if event.TaskID == "agent-fixed" && event.Snapshot.Status == StatusCompleted {
				finished.Add(1)
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if id != "agent-fixed" {
		t.Fatalf("id = %q, want preassigned", id)
	}
	eventA := waitDoneEvent(t, subA)
	eventB := waitDoneEvent(t, subB)
	if eventA.TaskID != id || eventB.TaskID != id {
		t.Fatal("subscribers did not receive task id")
	}
	if finished.Load() != 1 {
		t.Fatalf("finish callback count = %d", finished.Load())
	}
}

func waitDoneEvent(t *testing.T, ch <-chan DoneEvent) DoneEvent {
	t.Helper()
	select {
	case event := <-ch:
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for done event")
		return DoneEvent{}
	}
}

func TestManagerPanicRecoverAndStop(t *testing.T) {
	manager := NewManager(Options{IDSource: seqIDs(), Run: func(ctx context.Context, runner agent.Runner, conv *conversation.Conversation, prompt string) agent.CompletionResult {
		panic("boom")
	}})
	id, err := manager.Launch(context.Background(), LaunchInput{Prompt: "panic"})
	if err != nil {
		t.Fatal(err)
	}
	waitDone(t, manager, id)
	snap, _ := manager.Get(id)
	if snap.Status != StatusFailed || !strings.Contains(snap.Error, "boom") {
		t.Fatalf("panic snapshot = %#v", snap)
	}

	block := make(chan struct{})
	manager = NewManager(Options{IDSource: seqIDs(), Run: func(ctx context.Context, runner agent.Runner, conv *conversation.Conversation, prompt string) agent.CompletionResult {
		<-ctx.Done()
		close(block)
		return agent.CompletionResult{Stop: agent.Stop{Reason: agent.StopCanceled, Message: ctx.Err().Error()}}
	}})
	id, _ = manager.Launch(context.Background(), LaunchInput{Prompt: "block"})
	if !manager.Stop(id) {
		t.Fatal("Stop returned false")
	}
	waitDone(t, manager, id)
	<-block
	snap, _ = manager.Get(id)
	if snap.Status != StatusCancelled {
		t.Fatalf("stopped snapshot = %#v", snap)
	}
}

func TestManagerSendMessage(t *testing.T) {
	manager := NewManager(Options{IDSource: seqIDs(), Run: func(ctx context.Context, runner agent.Runner, conv *conversation.Conversation, prompt string) agent.CompletionResult {
		conv.AddUser(prompt)
		return agent.CompletionResult{Text: prompt, Stop: agent.Stop{Reason: agent.StopCompleted}}
	}})
	if _, err := manager.SendMessage(context.Background(), "missing", "hi"); err == nil {
		t.Fatal("expected missing name error")
	}
	id, _ := manager.Launch(context.Background(), LaunchInput{Name: "agent", Prompt: "one", Conversation: &conversation.Conversation{}})
	waitDone(t, manager, id)
	next, err := manager.SendMessage(context.Background(), "agent", "two")
	if err != nil {
		t.Fatal(err)
	}
	waitDone(t, manager, next)
	if next == id {
		t.Fatal("SendMessage should create a new task id")
	}
	snap, _ := manager.Get(next)
	if snap.Result != "two" {
		t.Fatalf("continued snapshot = %#v", snap)
	}
}

func TestManagerLaunchDoesNotInheritParentCancellation(t *testing.T) {
	allowFinish := make(chan struct{})
	manager := NewManager(Options{IDSource: seqIDs(), Run: func(ctx context.Context, runner agent.Runner, conv *conversation.Conversation, prompt string) agent.CompletionResult {
		select {
		case <-ctx.Done():
			return agent.CompletionResult{Stop: agent.Stop{Reason: agent.StopCanceled, Message: ctx.Err().Error()}}
		case <-allowFinish:
			return agent.CompletionResult{Text: "done", Stop: agent.Stop{Reason: agent.StopCompleted}}
		}
	}})
	parent, cancel := context.WithCancel(context.Background())
	id, err := manager.Launch(parent, LaunchInput{Prompt: "background"})
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case event := <-manager.SubscribeDone():
		t.Fatalf("task should not finish from parent cancellation: %+v", event)
	case <-time.After(30 * time.Millisecond):
	}
	close(allowFinish)
	waitDone(t, manager, id)
	snap, _ := manager.Get(id)
	if snap.Status != StatusCompleted {
		t.Fatalf("snapshot = %#v", snap)
	}
}

func TestManagerPrepareCleanup(t *testing.T) {
	manager := NewManager(Options{
		IDSource: seqIDs(),
		Run: func(ctx context.Context, runner agent.Runner, conv *conversation.Conversation, prompt string) agent.CompletionResult {
			return agent.CompletionResult{Text: prompt + " run", Stop: agent.Stop{Reason: agent.StopCompleted}}
		},
	})
	id, err := manager.Launch(context.Background(), LaunchInput{
		Prompt: "task",
		Prepare: func(ctx context.Context, runner agent.Runner, prompt string) (agent.Runner, string, agent.AgentCleanupFunc, error) {
			return runner, prompt + " prepared", func(ctx context.Context, result string) string {
				return result + " cleaned"
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitDone(t, manager, id)
	snap, _ := manager.Get(id)
	if snap.Result != "task prepared run cleaned" {
		t.Fatalf("snapshot = %#v", snap)
	}
}

func TestManagerPrepareRunsAsynchronously(t *testing.T) {
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	var running atomic.Int32
	manager := NewManager(Options{
		IDSource: atomicSeqIDs(),
		Run: func(ctx context.Context, runner agent.Runner, conv *conversation.Conversation, prompt string) agent.CompletionResult {
			return agent.CompletionResult{Text: prompt, Stop: agent.Stop{Reason: agent.StopCompleted}}
		},
	})
	prepare := func(ctx context.Context, runner agent.Runner, prompt string) (agent.Runner, string, agent.AgentCleanupFunc, error) {
		running.Add(1)
		started <- struct{}{}
		<-release
		return runner, prompt, nil, nil
	}
	id1, err := manager.Launch(context.Background(), LaunchInput{Prompt: "one", Prepare: prepare})
	if err != nil {
		t.Fatal(err)
	}
	id2, err := manager.Launch(context.Background(), LaunchInput{Prompt: "two", Prepare: prepare})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	<-started
	if running.Load() != 2 {
		t.Fatalf("prepare running = %d", running.Load())
	}
	close(release)
	waitStatus(t, manager, id1, StatusCompleted)
	waitStatus(t, manager, id2, StatusCompleted)
}

func seqIDs() IDSource {
	var n int
	return func() string {
		n++
		return "task-test-" + string(rune('0'+n))
	}
}

func atomicSeqIDs() IDSource {
	var n atomic.Int32
	return func() string {
		return "task-test-" + string(rune('0'+n.Add(1)))
	}
}

func waitDone(t *testing.T, manager *Manager, id string) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		select {
		case event := <-manager.SubscribeDone():
			if event.TaskID == id {
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s", id)
		}
	}
}

func waitStatus(t *testing.T, manager *Manager, id string, status Status) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snap, ok := manager.Get(id)
		if ok && snap.Status == status {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	snap, _ := manager.Get(id)
	t.Fatalf("timed out waiting for %s to become %s: %#v", id, status, snap)
}
