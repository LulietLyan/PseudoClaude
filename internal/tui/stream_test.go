package tui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"PseudoClaude/internal/agent"
	"PseudoClaude/internal/config"
	"PseudoClaude/internal/llm"
	"PseudoClaude/internal/tools"

	tea "charm.land/bubbletea/v2"
)

func TestNewStartsTextareaFocusedAndAcceptsInput(t *testing.T) {
	model := New(nil, t.TempDir(), nil)
	if !model.textarea.Focused() {
		t.Fatal("textarea should start focused")
	}
	next, _ := model.updateIdle(tea.KeyPressMsg{Code: 'h', Text: "h"})
	model = next.(Model)
	next, _ = model.updateIdle(tea.KeyPressMsg{Code: 'i', Text: "i"})
	model = next.(Model)
	if model.textarea.Value() != "hi" {
		t.Fatalf("textarea value = %q, want hi", model.textarea.Value())
	}
}

func TestBannerUsesSmallLogoInSmallViewport(t *testing.T) {
	model := New(nil, t.TempDir(), nil)
	model.width = 30
	model.height = 8
	banner := model.bannerView()
	if !strings.Contains(banner, "PseudoClaude") {
		t.Fatalf("banner missing logo text: %q", banner)
	}
	if strings.Contains(banner, "terminal agent") {
		t.Fatalf("small banner should use tiny logo, got: %q", banner)
	}
}

func TestBannerCentersFileLogoAndMeta(t *testing.T) {
	model := New(nil, t.TempDir(), nil)
	model.width = 90
	model.height = 24
	banner := model.bannerView()
	if !strings.Contains(banner, "██████╗") {
		t.Fatalf("banner missing file logo: %q", banner)
	}
	if !strings.Contains(banner, "PseudoClaude v"+Version) || !strings.Contains(banner, "Ready. Start") {
		t.Fatalf("banner missing centered meta lines: %q", banner)
	}
	lines := strings.Split(banner, "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], " ") {
		t.Fatalf("first logo line should be centered with left padding: %q", banner)
	}
}

func TestViewDoesNotRedrawBannerWhileTypingOrAfterReply(t *testing.T) {
	model := New([]config.ProviderConfig{}, t.TempDir(), nil)
	model.width = 90
	model.height = 24
	next, _ := model.updateIdle(tea.KeyPressMsg{Code: 'h', Text: "h"})
	model = next.(Model)
	view := model.view()
	if strings.Contains(view, "██████╗") {
		t.Fatalf("banner should not be part of redraw view while typing: %q", view)
	}

	model.provider = fakeProvider{events: []llm.StreamEvent{{Text: "hello"}, {Done: true}}}
	model.runner.Provider = model.provider
	next, _ = model.submit("hi")
	model = next.(Model)
	next, _ = model.updateStreaming(agentMsg{Type: agent.EventTextDelta, Text: "hello"})
	model = next.(Model)
	next, _ = model.updateStreaming(agentMsg{Type: agent.EventStop, Stop: &agent.Stop{Reason: agent.StopCompleted}})
	model = next.(Model)
	if model.state != stateIdle {
		t.Fatalf("state = %v, want idle", model.state)
	}
	view = model.view()
	if strings.Contains(view, "██████╗") {
		t.Fatalf("banner should not be redrawn after assistant reply: %q", view)
	}
}

func TestStreamingTextDeltasDoNotPanicAfterModelCopies(t *testing.T) {
	model := New([]config.ProviderConfig{}, t.TempDir(), nil)
	model.provider = fakeProvider{events: []llm.StreamEvent{{Text: "hello"}, {Done: true}}}
	model.runner.Provider = model.provider
	next, _ := model.submit("hi")
	model = next.(Model)

	next, _ = model.updateStreaming(agentMsg{Type: agent.EventTextDelta, Text: "hel"})
	model = next.(Model)
	next, _ = model.updateStreaming(agentMsg{Type: agent.EventTextDelta, Text: "lo"})
	model = next.(Model)

	if model.curReply != "hello" {
		t.Fatalf("curReply = %q, want hello", model.curReply)
	}
}

func TestViewDoesNotInsertLargeGapBeforeInput(t *testing.T) {
	model := New(nil, t.TempDir(), nil)
	model.width = 80
	model.height = 30
	view := model.view()
	if strings.Contains(view, "\n\n\n\n") {
		t.Fatalf("view has excessive vertical gap: %q", view)
	}
}

type fakeProvider struct {
	events []llm.StreamEvent
	defs   []tools.Definition
	msgs   []llm.Message
}

func (f fakeProvider) Name() string  { return "fake" }
func (f fakeProvider) Model() string { return "fake-model" }
func (f fakeProvider) Stream(_ context.Context, msgs []llm.Message, defs []tools.Definition) <-chan llm.StreamEvent {
	ch := make(chan llm.StreamEvent, len(f.events))
	for _, event := range f.events {
		ch <- event
	}
	close(ch)
	return ch
}

func TestAgentEventToolResultReturnsIdleAfterStop(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	registry, err := tools.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	model := New([]config.ProviderConfig{}, dir, registry)
	model.provider = fakeProvider{events: []llm.StreamEvent{{ToolCall: &llm.ToolCall{ID: "call_1", Name: "read_file", Arguments: json.RawMessage(`{"path":"note.txt"}`)}}, {Text: "done"}, {Done: true}}}
	model.runner.Provider = model.provider

	next, _ := model.submit("read")
	model = next.(Model)
	next, _ = model.updateStreaming(agentMsg{Type: agent.EventToolCallStart, ToolCall: &llm.ToolCall{ID: "call_1", Name: "read_file"}})
	model = next.(Model)
	result := tools.Success("read_file", "hello", nil)
	next, _ = model.updateStreaming(agentMsg{Type: agent.EventToolResult, ToolResult: &agent.ToolResult{
		Call:   llm.ToolCall{ID: "call_1", Name: "read_file"},
		Result: result,
	}})
	model = next.(Model)
	next, _ = model.updateStreaming(agentMsg{Type: agent.EventTextDelta, Text: "done"})
	model = next.(Model)
	next, _ = model.updateStreaming(agentMsg{Type: agent.EventStop, Stop: &agent.Stop{Reason: agent.StopCompleted}})
	model = next.(Model)
	if model.state != stateIdle {
		t.Fatalf("state = %v, want idle", model.state)
	}
	if model.curTool != nil {
		t.Fatalf("curTool = %+v, want nil", model.curTool)
	}
}

func TestPlanAndDoInputHandling(t *testing.T) {
	model := New([]config.ProviderConfig{}, t.TempDir(), nil)
	model.provider = fakeProvider{events: []llm.StreamEvent{{Text: "plan text"}, {Done: true}}}
	model.runner.Provider = model.provider

	req, printable, err := model.requestForInput("/plan change the thing")
	if err != nil {
		t.Fatal(err)
	}
	if req.Mode != agent.ModePlan || req.PlanTask != "change the thing" || printable != "/plan change the thing" {
		t.Fatalf("plan request = %+v printable=%q", req, printable)
	}
	if _, _, err := model.requestForInput("/do"); err == nil {
		t.Fatal("expected missing plan error")
	}
	model.lastPlan = &planState{task: "change the thing", text: "plan text"}
	req, _, err = model.requestForInput("/do")
	if err != nil {
		t.Fatal(err)
	}
	if req.Mode != agent.ModeDo || req.PlanTask != "change the thing" || req.PlanText != "plan text" {
		t.Fatalf("do request = %+v", req)
	}
}

func TestPlanModePersistsForPlainFollowupInput(t *testing.T) {
	model := New([]config.ProviderConfig{}, t.TempDir(), nil)
	model.provider = fakeProvider{events: []llm.StreamEvent{{Text: "plan text"}, {Done: true}}}
	model.runner.Provider = model.provider
	model.planMode = true

	req, printable, err := model.requestForInput("我要做个电商系统")
	if err != nil {
		t.Fatal(err)
	}
	if req.Mode != agent.ModePlan || req.PlanTask != "我要做个电商系统" || printable != "我要做个电商系统" {
		t.Fatalf("request = %+v printable=%q", req, printable)
	}
}

func TestPlanStateSavedAndCleared(t *testing.T) {
	model := New([]config.ProviderConfig{}, t.TempDir(), nil)
	model.conv.AddUser("Plan mode. Create an implementation plan.\n\nTask:\nchange it")
	model.saveOrClearPlan("plan text")
	if model.lastPlan == nil || model.lastPlan.task != "change it" || model.lastPlan.text != "plan text" {
		t.Fatalf("lastPlan = %+v", model.lastPlan)
	}
	model.conv.AddUser("Execution mode. Carry out the following task.\n\nOriginal task:\nchange it\n\nPlan:\nplan text")
	model.saveOrClearPlan("done")
	if model.lastPlan != nil {
		t.Fatalf("lastPlan should be cleared: %+v", model.lastPlan)
	}
}
