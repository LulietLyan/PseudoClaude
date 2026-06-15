package agent

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"PseudoClaude/internal/conversation"
	"PseudoClaude/internal/llm"
	"PseudoClaude/internal/tools"
)

type fakeProvider struct {
	mu       sync.Mutex
	streams  [][]llm.StreamEvent
	messages [][]llm.Message
	defs     [][]tools.Definition
}

func (f *fakeProvider) Name() string  { return "fake" }
func (f *fakeProvider) Model() string { return "fake-model" }

func (f *fakeProvider) Stream(_ context.Context, msgs []llm.Message, defs []tools.Definition) <-chan llm.StreamEvent {
	f.mu.Lock()
	f.messages = append(f.messages, msgs)
	f.defs = append(f.defs, defs)
	index := len(f.messages) - 1
	var events []llm.StreamEvent
	if index < len(f.streams) {
		events = f.streams[index]
	}
	f.mu.Unlock()

	ch := make(chan llm.StreamEvent, len(events))
	for _, event := range events {
		ch <- event
	}
	close(ch)
	return ch
}

type scriptedTool struct {
	name    string
	safety  tools.Safety
	delay   time.Duration
	order   *[]string
	content string
}

type fakeExecTool struct {
	name     string
	safety   tools.Safety
	executed *bool
}

func (f fakeExecTool) Definition() tools.Definition {
	return tools.Definition{Name: f.name, Description: "fake exec", InputSchema: map[string]any{"type": "object"}, Safety: f.safety}
}

func (f fakeExecTool) Execute(ctx context.Context, input json.RawMessage, env tools.Env) tools.Result {
	if f.executed != nil {
		*f.executed = true
	}
	return tools.Success(f.name, "executed", nil)
}

func (s scriptedTool) Definition() tools.Definition {
	return tools.Definition{Name: s.name, Description: "scripted", InputSchema: map[string]any{"type": "object"}, Safety: s.safety}
}

func (s scriptedTool) Execute(ctx context.Context, input json.RawMessage, env tools.Env) tools.Result {
	if s.delay > 0 {
		select {
		case <-ctx.Done():
			return tools.Failure(s.name, "timeout", ctx.Err().Error(), nil)
		case <-time.After(s.delay):
		}
	}
	if s.order != nil {
		*s.order = append(*s.order, s.name)
	}
	content := s.content
	if content == "" {
		content = s.name + " ok"
	}
	return tools.Success(s.name, content, nil)
}

func collectEvents(ch <-chan Event) []Event {
	var events []Event
	for event := range ch {
		events = append(events, event)
	}
	return events
}

func TestCollectorPublishesDeltasAndCollectsOutput(t *testing.T) {
	stream := make(chan llm.StreamEvent, 4)
	stream <- llm.StreamEvent{Text: "hel"}
	stream <- llm.StreamEvent{Text: "lo"}
	stream <- llm.StreamEvent{ToolCall: &llm.ToolCall{ID: "call_1", Name: "read_file", Arguments: json.RawMessage(`{}`)}}
	stream <- llm.StreamEvent{Usage: &llm.Usage{InputTokens: 1, OutputTokens: 2, TotalTokens: 3}, Done: true}
	close(stream)

	events := make(chan Event, 8)
	collector := &streamCollector{}
	out, err := collector.collect(context.Background(), 1, stream, events)
	close(events)
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "hello" {
		t.Fatalf("text = %q", out.Text)
	}
	if len(out.ToolCalls) != 1 || out.ToolCalls[0].ID != "call_1" {
		t.Fatalf("tool calls = %+v", out.ToolCalls)
	}
	if out.Usage == nil || out.Usage.TotalTokens != 3 {
		t.Fatalf("usage = %+v", out.Usage)
	}
	var delta string
	seenUsage := false
	for event := range events {
		if event.Type == EventTextDelta {
			delta += event.Text
		}
		if event.Type == EventUsage {
			seenUsage = true
		}
	}
	if delta != out.Text || !seenUsage {
		t.Fatalf("delta = %q seenUsage=%v", delta, seenUsage)
	}
}

func TestRunnerCompletesAfterMultipleToolRounds(t *testing.T) {
	provider := &fakeProvider{streams: [][]llm.StreamEvent{
		{{ToolCall: &llm.ToolCall{ID: "call_1", Name: "read_file", Arguments: json.RawMessage(`{}`)}}, {Done: true}},
		{{ToolCall: &llm.ToolCall{ID: "call_2", Name: "search_code", Arguments: json.RawMessage(`{}`)}}, {Done: true}},
		{{Text: "done"}, {Done: true}},
	}}
	registry, err := tools.NewRegistry(
		scriptedTool{name: "read_file", safety: tools.SafetyReadOnly},
		scriptedTool{name: "search_code", safety: tools.SafetyReadOnly},
	)
	if err != nil {
		t.Fatal(err)
	}
	var conv conversation.Conversation
	events := collectEvents(Runner{Provider: provider, Registry: registry}.Run(context.Background(), Request{
		Mode:         ModeChat,
		UserText:     "do it",
		Conversation: &conv,
	}))
	stop := lastStop(t, events)
	if stop.Reason != StopCompleted || stop.Iterations != 3 {
		t.Fatalf("stop = %+v", stop)
	}
	if len(provider.messages) != 3 {
		t.Fatalf("provider calls = %d, want 3", len(provider.messages))
	}
	msgs := conv.Messages()
	if len(msgs) != 6 {
		t.Fatalf("message count = %d, want 6: %+v", len(msgs), msgs)
	}
	if msgs[5].Role != "assistant" || msgs[5].Content != "done" {
		t.Fatalf("final message = %+v", msgs[5])
	}
}

func TestRunnerHandlesMultipleToolCallsInOrder(t *testing.T) {
	provider := &fakeProvider{streams: [][]llm.StreamEvent{
		{
			{ToolCall: &llm.ToolCall{ID: "slow", Name: "slow_read", Arguments: json.RawMessage(`{}`)}},
			{ToolCall: &llm.ToolCall{ID: "fast", Name: "fast_read", Arguments: json.RawMessage(`{}`)}},
			{Done: true},
		},
		{{Text: "done"}, {Done: true}},
	}}
	var order []string
	registry, err := tools.NewRegistry(
		scriptedTool{name: "slow_read", safety: tools.SafetyReadOnly, delay: 30 * time.Millisecond, order: &order},
		scriptedTool{name: "fast_read", safety: tools.SafetyReadOnly, order: &order},
	)
	if err != nil {
		t.Fatal(err)
	}
	var conv conversation.Conversation
	events := collectEvents(Runner{Provider: provider, Registry: registry}.Run(context.Background(), Request{UserText: "tools", Conversation: &conv}))
	if lastStop(t, events).Reason != StopCompleted {
		t.Fatalf("events = %+v", events)
	}
	msgs := conv.Messages()
	if len(msgs) < 4 || msgs[2].ToolResult == nil || msgs[3].ToolResult == nil {
		t.Fatalf("messages = %+v", msgs)
	}
	if msgs[2].ToolResult.CallID != "slow" || msgs[3].ToolResult.CallID != "fast" {
		t.Fatalf("tool result order = %s, %s", msgs[2].ToolResult.CallID, msgs[3].ToolResult.CallID)
	}
	if len(order) != 2 || order[0] != "fast_read" || order[1] != "slow_read" {
		t.Fatalf("expected concurrent completion order fast then slow, got %+v", order)
	}
}

func TestSideEffectToolsRunSerially(t *testing.T) {
	var order []string
	registry, err := tools.NewRegistry(
		scriptedTool{name: "write_a", safety: tools.SafetySideEffect, order: &order},
		scriptedTool{name: "write_b", safety: tools.SafetySideEffect, order: &order},
	)
	if err != nil {
		t.Fatal(err)
	}
	calls := []llm.ToolCall{
		{ID: "a", Name: "write_a", Arguments: json.RawMessage(`{}`)},
		{ID: "b", Name: "write_b", Arguments: json.RawMessage(`{}`)},
	}
	events := make(chan Event, 16)
	results, err := executeToolCalls(context.Background(), registry, tools.Env{}, 1, calls, events, toolExecutionOptions{})
	close(events)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].Call.ID != "a" || results[1].Call.ID != "b" {
		t.Fatalf("results = %+v", results)
	}
	if len(order) != 2 || order[0] != "write_a" || order[1] != "write_b" {
		t.Fatalf("order = %+v", order)
	}
}

func TestPlanModeRejectsSideEffectToolIfModelRequestsIt(t *testing.T) {
	provider := &fakeProvider{streams: [][]llm.StreamEvent{
		{{ToolCall: &llm.ToolCall{ID: "call_1", Name: "write_file", Arguments: json.RawMessage(`{}`)}}, {Done: true}},
		{{Text: "I need requirements first."}, {Done: true}},
	}}
	executed := false
	registry, err := tools.NewRegistry(
		scriptedTool{name: "read_file", safety: tools.SafetyReadOnly},
		fakeExecTool{name: "write_file", safety: tools.SafetySideEffect, executed: &executed},
	)
	if err != nil {
		t.Fatal(err)
	}
	var conv conversation.Conversation
	events := collectEvents(Runner{Provider: provider, Registry: registry}.Run(context.Background(), Request{
		Mode:         ModePlan,
		PlanTask:     "make a store",
		Conversation: &conv,
	}))
	if lastStop(t, events).Reason != StopCompleted {
		t.Fatalf("events = %+v", events)
	}
	if executed {
		t.Fatal("side effect tool executed in plan mode")
	}
	msgs := conv.Messages()
	var found bool
	for _, msg := range msgs {
		if msg.ToolResult != nil && strings.Contains(msg.ToolResult.Content, "tool_not_allowed") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected tool_not_allowed result in messages: %+v", msgs)
	}
}

func TestRunnerStopsAtMaxIterations(t *testing.T) {
	provider := &fakeProvider{streams: [][]llm.StreamEvent{
		{{ToolCall: &llm.ToolCall{ID: "call_1", Name: "read_file", Arguments: json.RawMessage(`{}`)}}, {Done: true}},
	}}
	registry, _ := tools.NewRegistry(scriptedTool{name: "read_file", safety: tools.SafetyReadOnly})
	events := collectEvents(Runner{
		Provider: provider,
		Registry: registry,
		Config:   Config{MaxIterations: 1},
	}.Run(context.Background(), Request{UserText: "loop", Conversation: &conversation.Conversation{}}))
	stop := lastStop(t, events)
	if stop.Reason != StopMaxIterations || len(provider.messages) != 1 {
		t.Fatalf("stop=%+v provider calls=%d", stop, len(provider.messages))
	}
}

func TestRunnerStopsAtUnknownToolLimitAfterRecordingErrors(t *testing.T) {
	provider := &fakeProvider{streams: [][]llm.StreamEvent{
		{{ToolCall: &llm.ToolCall{ID: "missing_1", Name: "missing", Arguments: json.RawMessage(`{}`)}}, {Done: true}},
		{{ToolCall: &llm.ToolCall{ID: "missing_2", Name: "missing", Arguments: json.RawMessage(`{}`)}}, {Done: true}},
	}}
	registry, _ := tools.NewRegistry()
	var conv conversation.Conversation
	events := collectEvents(Runner{
		Provider: provider,
		Registry: registry,
		Config:   Config{MaxIterations: 5, MaxUnknownToolCalls: 2},
	}.Run(context.Background(), Request{UserText: "loop", Conversation: &conv}))
	if lastStop(t, events).Reason != StopUnknownToolLimit {
		t.Fatalf("events = %+v", events)
	}
	var errors int
	for _, msg := range conv.Messages() {
		if msg.ToolResult != nil && msg.ToolResult.IsError {
			errors++
		}
	}
	if errors != 2 {
		t.Fatalf("tool errors = %d, want 2", errors)
	}
}

func TestRunnerPlanAndDoModesSelectToolsAndPrompt(t *testing.T) {
	provider := &fakeProvider{streams: [][]llm.StreamEvent{
		{{Text: "plan"}, {Done: true}},
		{{Text: "done"}, {Done: true}},
	}}
	registry, err := tools.NewRegistry(
		scriptedTool{name: "read_file", safety: tools.SafetyReadOnly},
		scriptedTool{name: "write_file", safety: tools.SafetySideEffect},
	)
	if err != nil {
		t.Fatal(err)
	}
	runner := Runner{Provider: provider, Registry: registry}
	collectEvents(runner.Run(context.Background(), Request{Mode: ModePlan, PlanTask: "change x", Conversation: &conversation.Conversation{}}))
	if len(provider.defs[0]) != 1 || provider.defs[0][0].Name != "read_file" {
		t.Fatalf("plan defs = %+v", provider.defs[0])
	}
	collectEvents(runner.Run(context.Background(), Request{Mode: ModeDo, PlanTask: "change x", PlanText: "plan text", Conversation: &conversation.Conversation{}}))
	if len(provider.defs[1]) != 2 {
		t.Fatalf("do defs = %+v", provider.defs[1])
	}
	if got := provider.messages[1][0].Content; !containsAll(got, "change x", "plan text") {
		t.Fatalf("do prompt = %q", got)
	}
}

func TestRunnerStreamErrorStop(t *testing.T) {
	provider := &fakeProvider{streams: [][]llm.StreamEvent{{{Err: context.Canceled}}}}
	events := collectEvents(Runner{Provider: provider, Registry: &tools.Registry{}}.Run(context.Background(), Request{UserText: "hi", Conversation: &conversation.Conversation{}}))
	if lastStop(t, events).Reason != StopStreamError {
		t.Fatalf("events = %+v", events)
	}
}

func lastStop(t *testing.T, events []Event) Stop {
	t.Helper()
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Stop != nil {
			return *events[i].Stop
		}
	}
	t.Fatalf("no stop event: %+v", events)
	return Stop{}
}

func containsAll(s string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(s, part) {
			return false
		}
	}
	return true
}
