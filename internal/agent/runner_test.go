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
	requests []llm.Request
}

func (f *fakeProvider) Name() string  { return "fake" }
func (f *fakeProvider) Model() string { return "fake-model" }

func (f *fakeProvider) Stream(_ context.Context, req llm.Request) <-chan llm.StreamEvent {
	f.mu.Lock()
	f.requests = append(f.requests, req)
	index := len(f.requests) - 1
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
	order   *recordedOrder
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
		s.order.add(s.name)
	}
	content := s.content
	if content == "" {
		content = s.name + " ok"
	}
	return tools.Success(s.name, content, nil)
}

type recordedOrder struct {
	mu    sync.Mutex
	names []string
}

func (r *recordedOrder) add(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.names = append(r.names, name)
}

func (r *recordedOrder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.names...)
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
	if len(provider.requests) != 3 {
		t.Fatalf("provider calls = %d, want 3", len(provider.requests))
	}
	if provider.requests[0].System.Stable == "" || provider.requests[0].System.Environment == "" {
		t.Fatalf("system request = %+v", provider.requests[0].System)
	}
	if provider.requests[0].System.Stable != provider.requests[1].System.Stable {
		t.Fatal("stable system prompt changed across rounds")
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
	order := &recordedOrder{}
	registry, err := tools.NewRegistry(
		scriptedTool{name: "slow_read", safety: tools.SafetyReadOnly, delay: 30 * time.Millisecond, order: order},
		scriptedTool{name: "fast_read", safety: tools.SafetyReadOnly, order: order},
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
	gotOrder := order.snapshot()
	if len(gotOrder) != 2 || gotOrder[0] != "fast_read" || gotOrder[1] != "slow_read" {
		t.Fatalf("expected concurrent completion order fast then slow, got %+v", gotOrder)
	}
}

func TestSideEffectToolsRunSerially(t *testing.T) {
	order := &recordedOrder{}
	registry, err := tools.NewRegistry(
		scriptedTool{name: "write_a", safety: tools.SafetySideEffect, order: order},
		scriptedTool{name: "write_b", safety: tools.SafetySideEffect, order: order},
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
	gotOrder := order.snapshot()
	if len(gotOrder) != 2 || gotOrder[0] != "write_a" || gotOrder[1] != "write_b" {
		t.Fatalf("order = %+v", gotOrder)
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
	if stop.Reason != StopMaxIterations || len(provider.requests) != 1 {
		t.Fatalf("stop=%+v provider calls=%d", stop, len(provider.requests))
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
	if len(provider.requests[0].Tools) != 1 || provider.requests[0].Tools[0].Name != "read_file" {
		t.Fatalf("plan defs = %+v", provider.requests[0].Tools)
	}
	collectEvents(runner.Run(context.Background(), Request{Mode: ModeDo, PlanTask: "change x", PlanText: "plan text", Conversation: &conversation.Conversation{}}))
	if len(provider.requests[1].Tools) != 2 {
		t.Fatalf("do defs = %+v", provider.requests[1].Tools)
	}
	if got := provider.requests[1].Messages[0].Content; !containsAll(got, "change x", "plan text") {
		t.Fatalf("do prompt = %q", got)
	}
	if provider.requests[0].System.Stable == "" || provider.requests[1].System.Stable == "" || provider.requests[0].System.Stable != provider.requests[1].System.Stable {
		t.Fatalf("stable prompts = %q / %q", provider.requests[0].System.Stable, provider.requests[1].System.Stable)
	}
	if !containsAll(provider.requests[0].System.Environment, "provider", "fake", "fake-model") {
		t.Fatalf("environment = %q", provider.requests[0].System.Environment)
	}
	if !strings.Contains(provider.requests[0].Reminder, "You are in plan mode") {
		t.Fatalf("plan reminder = %q", provider.requests[0].Reminder)
	}
	if provider.requests[1].Reminder != "" {
		t.Fatalf("do reminder = %q", provider.requests[1].Reminder)
	}
}

func TestPlanReminderFrequencyAndNotPersisted(t *testing.T) {
	provider := &fakeProvider{streams: [][]llm.StreamEvent{
		{{ToolCall: &llm.ToolCall{ID: "call_1", Name: "read_file", Arguments: json.RawMessage(`{}`)}}, {Done: true}},
		{{ToolCall: &llm.ToolCall{ID: "call_2", Name: "read_file", Arguments: json.RawMessage(`{}`)}}, {Done: true}},
		{{ToolCall: &llm.ToolCall{ID: "call_3", Name: "read_file", Arguments: json.RawMessage(`{}`)}}, {Done: true}},
		{{ToolCall: &llm.ToolCall{ID: "call_4", Name: "read_file", Arguments: json.RawMessage(`{}`)}}, {Done: true}},
		{{Text: "plan ready"}, {Done: true}},
	}}
	registry, err := tools.NewRegistry(scriptedTool{name: "read_file", safety: tools.SafetyReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	var conv conversation.Conversation
	events := collectEvents(Runner{Provider: provider, Registry: registry}.Run(context.Background(), Request{
		Mode:         ModePlan,
		PlanTask:     "inspect",
		Conversation: &conv,
	}))
	if lastStop(t, events).Reason != StopCompleted {
		t.Fatalf("events = %+v", events)
	}
	if len(provider.requests) != 5 {
		t.Fatalf("provider calls = %d", len(provider.requests))
	}
	if !strings.Contains(provider.requests[0].Reminder, "You are in plan mode") {
		t.Fatalf("round 1 reminder = %q", provider.requests[0].Reminder)
	}
	if !strings.Contains(provider.requests[1].Reminder, "Still in plan mode") {
		t.Fatalf("round 2 reminder = %q", provider.requests[1].Reminder)
	}
	if !strings.Contains(provider.requests[4].Reminder, "You are in plan mode") {
		t.Fatalf("round 5 reminder = %q", provider.requests[4].Reminder)
	}
	for _, msg := range conv.Messages() {
		if strings.Contains(msg.Content, "<system-reminder>") {
			t.Fatalf("reminder persisted in conversation: %+v", conv.Messages())
		}
	}
}

func TestRunnerTransmitsCacheUsage(t *testing.T) {
	provider := &fakeProvider{streams: [][]llm.StreamEvent{{
		{Usage: &llm.Usage{InputTokens: 1, OutputTokens: 2, TotalTokens: 36, CacheWrite: 11, CacheRead: 22}},
		{Text: "done"},
		{Done: true},
	}}}
	events := collectEvents(Runner{Provider: provider, Registry: &tools.Registry{}}.Run(context.Background(), Request{
		UserText:     "hi",
		Conversation: &conversation.Conversation{},
	}))
	var usage *llm.Usage
	for _, event := range events {
		if event.Type == EventUsage {
			usage = event.Usage
		}
	}
	if usage == nil || usage.CacheWrite != 11 || usage.CacheRead != 22 {
		t.Fatalf("usage = %+v events=%+v", usage, events)
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
