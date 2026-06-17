package compact

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"PseudoClaude/internal/conversation"
	"PseudoClaude/internal/llm"
)

type fakeSummaryProvider struct {
	requests []llm.Request
	streams  [][]llm.StreamEvent
}

func (f *fakeSummaryProvider) Name() string  { return "fake" }
func (f *fakeSummaryProvider) Model() string { return "fake-model" }

func (f *fakeSummaryProvider) Stream(_ context.Context, req llm.Request) <-chan llm.StreamEvent {
	f.requests = append(f.requests, req)
	index := len(f.requests) - 1
	var events []llm.StreamEvent
	if index < len(f.streams) {
		events = f.streams[index]
	}
	ch := make(chan llm.StreamEvent, len(events))
	for _, event := range events {
		ch <- event
	}
	close(ch)
	return ch
}

func TestUsageTokens(t *testing.T) {
	if tokens, ok := UsageTokens(&llm.Usage{TotalTokens: 42, InputTokens: 1}); !ok || tokens != 42 {
		t.Fatalf("total tokens = %d ok=%v", tokens, ok)
	}
	if tokens, ok := UsageTokens(&llm.Usage{InputTokens: 1, OutputTokens: 2, CacheRead: 3, CacheWrite: 4}); !ok || tokens != 10 {
		t.Fatalf("summed tokens = %d ok=%v", tokens, ok)
	}
	if _, ok := UsageTokens(nil); ok {
		t.Fatal("nil usage should be invalid")
	}
}

func TestOffloadSingleLargeToolResultAndReuse(t *testing.T) {
	rt, err := NewRuntime(t.TempDir(), DefaultOpenAIContextWindow)
	if err != nil {
		t.Fatal(err)
	}
	large := strings.Repeat("a", SingleToolResultLimitBytes+1)
	msgs := []llm.Message{{Role: "user", ToolResult: &llm.ToolResult{CallID: "call_1", Name: "read_file", Content: large}}}

	out := OffloadToolResults(msgs, rt)
	if out.Err != nil {
		t.Fatal(out.Err)
	}
	if !out.Changed || out.OffloadedCount != 1 {
		t.Fatalf("out = %+v", out)
	}
	got := out.Messages[0].ToolResult.Content
	if got == large || !strings.Contains(got, "original size") || !strings.Contains(got, "文件读取工具") {
		t.Fatalf("preview = %q", got[:min(len(got), 200)])
	}
	path := filepath.Join(rt.Snapshot().Session.SpillDir, "call_1.txt")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != large {
		t.Fatal("spilled content did not match original")
	}

	msgs[0].ToolResult.Content = strings.Repeat("b", SingleToolResultLimitBytes+10)
	again := OffloadToolResults(msgs, rt)
	if again.Messages[0].ToolResult.Content != got {
		t.Fatal("replacement preview was not reused")
	}
}

func TestOffloadFailureDoesNotFreezeDecision(t *testing.T) {
	rt, err := NewRuntime(t.TempDir(), DefaultOpenAIContextWindow)
	if err != nil {
		t.Fatal(err)
	}
	session := rt.Snapshot().Session
	if err := os.RemoveAll(session.SpillDir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(session.SpillDir, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	large := strings.Repeat("x", SingleToolResultLimitBytes+1)
	msgs := []llm.Message{{Role: "user", ToolResult: &llm.ToolResult{CallID: "call_1", Name: "read_file", Content: large}}}

	out := OffloadToolResults(msgs, rt)
	if out.Err == nil || out.Messages[0].ToolResult.Content != large {
		t.Fatalf("expected original content after spill failure: out=%+v err=%v", out, out.Err)
	}

	if err := os.Remove(session.SpillDir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(session.SpillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	again := OffloadToolResults(msgs, rt)
	if again.Err != nil {
		t.Fatal(again.Err)
	}
	if again.Messages[0].ToolResult.Content == large || !strings.Contains(again.Messages[0].ToolResult.Content, "[content offloaded]") {
		t.Fatalf("expected retry to offload after directory fixed: %.80q", again.Messages[0].ToolResult.Content)
	}
}

func TestOffloadAggregateLimitChoosesLargestResults(t *testing.T) {
	rt, err := NewRuntime(t.TempDir(), DefaultOpenAIContextWindow)
	if err != nil {
		t.Fatal(err)
	}
	msgs := []llm.Message{
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "a", Name: "t"}, {ID: "b", Name: "t"}, {ID: "c", Name: "t"}, {ID: "d", Name: "t"}, {ID: "e", Name: "t"}}},
		{Role: "user", ToolResult: &llm.ToolResult{CallID: "a", Name: "t", Content: strings.Repeat("a", 49000)}},
		{Role: "user", ToolResult: &llm.ToolResult{CallID: "b", Name: "t", Content: strings.Repeat("b", 48000)}},
		{Role: "user", ToolResult: &llm.ToolResult{CallID: "c", Name: "t", Content: strings.Repeat("c", 47000)}},
		{Role: "user", ToolResult: &llm.ToolResult{CallID: "d", Name: "t", Content: strings.Repeat("d", 46000)}},
		{Role: "user", ToolResult: &llm.ToolResult{CallID: "e", Name: "t", Content: strings.Repeat("e", 45000)}},
	}
	out := OffloadToolResults(msgs, rt)
	if out.Err != nil {
		t.Fatal(out.Err)
	}
	if !strings.Contains(out.Messages[1].ToolResult.Content, "[content offloaded]") {
		t.Fatalf("largest result was not offloaded: %.40q", out.Messages[1].ToolResult.Content)
	}
	if strings.Contains(out.Messages[2].ToolResult.Content, "[content offloaded]") || strings.Contains(out.Messages[3].ToolResult.Content, "[content offloaded]") || strings.Contains(out.Messages[4].ToolResult.Content, "[content offloaded]") || strings.Contains(out.Messages[5].ToolResult.Content, "[content offloaded]") {
		t.Fatalf("unexpected aggregate offloads: %+v", out.Messages)
	}
}

func TestExtractSummaryDropsAnalysis(t *testing.T) {
	raw := "<analysis>private notes</analysis>\n<summary>public summary</summary>"
	if got := extractSummary(raw); got != "public summary" {
		t.Fatalf("summary = %q", got)
	}
}

func TestSelectRecentDoesNotSplitToolCallAndResult(t *testing.T) {
	msgs := []llm.Message{
		{Role: "user", Content: "old"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "call_1", Name: "read_file"}}},
		{Role: "user", ToolResult: &llm.ToolResult{CallID: "call_1", Name: "read_file", Content: strings.Repeat("x", int(EstimateCharsPerToken*RecentKeepTokens))}},
		{Role: "assistant", Content: "done"},
		{Role: "user", Content: "next"},
		{Role: "assistant", Content: "ok"},
		{Role: "user", Content: "again"},
	}
	recent := SelectRecent(msgs)
	if len(recent) < 2 || len(recent[0].ToolCalls) == 0 || recent[1].ToolResult == nil {
		t.Fatalf("recent split tool pair: %+v", recent)
	}
}

func TestManageContextAutoBreakerTripsAndManualBypasses(t *testing.T) {
	rt, err := NewRuntime(t.TempDir(), 1000)
	if err != nil {
		t.Fatal(err)
	}
	provider := &fakeSummaryProvider{streams: [][]llm.StreamEvent{
		{{Err: errors.New("summary failed")}},
		{{Err: errors.New("summary failed")}},
		{{Err: errors.New("summary failed")}},
		{{Text: "<analysis>x</analysis><summary>ok</summary>"}, {Done: true}},
	}}
	for i := 0; i < AutoFailureLimit; i++ {
		conv := largeConversation()
		_, err := ManageContext(context.Background(), ManageInput{Conversation: conv, Runtime: rt, Provider: provider, Trigger: TriggerAuto})
		if err == nil {
			t.Fatalf("attempt %d should fail", i+1)
		}
	}
	if !rt.AutoTripped() {
		t.Fatal("auto breaker should be tripped")
	}
	conv := largeConversation()
	out, err := ManageContext(context.Background(), ManageInput{Conversation: conv, Runtime: rt, Provider: provider, Trigger: TriggerAuto})
	if err != nil {
		t.Fatal(err)
	}
	if out.TriggeredLayer2 || len(provider.requests) != AutoFailureLimit {
		t.Fatalf("auto should be bypassed after trip: out=%+v requests=%d", out, len(provider.requests))
	}
	out, err = ForceCompact(context.Background(), ManageInput{Conversation: conv, Runtime: rt, Provider: provider, Trigger: TriggerManual})
	if err != nil {
		t.Fatal(err)
	}
	if !out.TriggeredLayer2 || len(provider.requests) != AutoFailureLimit+1 {
		t.Fatalf("manual did not bypass breaker: out=%+v requests=%d", out, len(provider.requests))
	}
}

func TestManageContextAutoCompactsBeforeOrdinaryRequest(t *testing.T) {
	rt, err := NewRuntime(t.TempDir(), 1000)
	if err != nil {
		t.Fatal(err)
	}
	provider := &fakeSummaryProvider{streams: [][]llm.StreamEvent{{{Text: "<analysis>x</analysis><summary>summary text</summary>"}, {Done: true}}}}
	conv := largeConversation()
	out, err := ManageContext(context.Background(), ManageInput{Conversation: conv, Runtime: rt, Provider: provider, Trigger: TriggerAuto})
	if err != nil {
		t.Fatal(err)
	}
	if !out.TriggeredLayer2 {
		t.Fatalf("output = %+v", out)
	}
	if len(provider.requests) != 1 || len(provider.requests[0].Tools) != 0 {
		t.Fatalf("summary request = %+v", provider.requests)
	}
	if !strings.Contains(conv.Messages()[0].Content, "summary text") {
		t.Fatalf("compacted messages = %+v", conv.Messages())
	}
}

func largeConversation() *conversation.Conversation {
	conv := &conversation.Conversation{}
	for i := 0; i < 6; i++ {
		conv.AddUser(strings.Repeat("u", 2000))
		conv.AddAssistant(strings.Repeat("a", 2000))
	}
	return conv
}
