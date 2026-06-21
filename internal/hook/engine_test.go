package hook

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type fakeExecutor struct {
	counts  atomic.Int32
	delay   time.Duration
	results map[string]ExecutionResult
}

func (f *fakeExecutor) run(ctx context.Context, rule Rule, payload Payload, blocking bool) ExecutionResult {
	f.counts.Add(1)
	if f.delay > 0 {
		select {
		case <-ctx.Done():
			return ExecutionResult{Err: ctx.Err()}
		case <-time.After(f.delay):
		}
	}
	if result, ok := f.results[rule.Name]; ok {
		return result
	}
	return ExecutionResult{}
}

func TestEngine(t *testing.T) {
	logs := []string{}
	exec := Executor{Logf: func(format string, args ...any) { logs = append(logs, formatString(format, args...)) }}
	rules := []Rule{
		{Name: "first", Event: EventPreUserMessage, Index: 0, Action: Action{Type: ActionPrompt, Prompt: &PromptAction{Text: "one"}}},
		{Name: "second", Event: EventPreUserMessage, Index: 1, Action: Action{Type: ActionPrompt, Prompt: &PromptAction{Text: "two"}}},
		{Name: "once", Event: EventStop, OnlyOnce: true, Index: 2, Action: Action{Type: ActionShell, Shell: &ShellAction{Command: "exit 0"}}},
	}
	engine := NewEngine(rules, exec, []string{"source"})
	got := engine.Dispatch(context.Background(), EventPreUserMessage, Payload{})
	if strings.Join(got.InjectedPrompts, ",") != "one,two" {
		t.Fatalf("prompts = %+v", got.InjectedPrompts)
	}
	engine.Dispatch(context.Background(), EventStop, Payload{})
	engine.Dispatch(context.Background(), EventStop, Payload{})
	engine.ResetForNewSession()
	engine.Dispatch(context.Background(), EventStop, Payload{})
	if len(engine.Sources()) != 1 || len(engine.Summaries()) != 3 || len(logs) != 0 {
		t.Fatalf("sources/summaries/logs = %+v %+v %+v", engine.Sources(), engine.Summaries(), logs)
	}
}

func TestEngineBlockShortCircuit(t *testing.T) {
	engine := NewEngine([]Rule{
		{Name: "block", Event: EventPreToolUse, Index: 0, Action: Action{Type: ActionShell, Shell: &ShellAction{Command: "echo nope >&2; exit 2"}}},
		{Name: "later", Event: EventPreToolUse, Index: 1, Action: Action{Type: ActionPrompt, Prompt: &PromptAction{Text: "later"}}},
	}, Executor{}, nil)
	got := engine.Dispatch(context.Background(), EventPreToolUse, Payload{})
	if !got.Blocked || got.BlockingHookName != "block" || got.Reason != "nope" || len(got.InjectedPrompts) != 0 {
		t.Fatalf("block result = %+v", got)
	}
}

func TestEngineAsyncAndFailure(t *testing.T) {
	done := make(chan struct{})
	engine := NewEngine([]Rule{
		{Name: "async", Event: EventPostToolUse, Async: true, Timeout: time.Second, Action: Action{Type: ActionShell, Shell: &ShellAction{Command: "sleep 0.05"}}},
		{Name: "fail", Event: EventPostToolUse, Action: Action{Type: ActionShell, Shell: &ShellAction{Command: "echo fail >&2; exit 1"}}},
	}, Executor{Logf: func(format string, args ...any) {
		if strings.Contains(formatString(format, args...), "fail") {
			close(done)
		}
	}}, nil)
	start := time.Now()
	got := engine.Dispatch(context.Background(), EventPostToolUse, Payload{})
	if got.Blocked || time.Since(start) > 40*time.Millisecond {
		t.Fatalf("async dispatch blocked: %+v elapsed=%s", got, time.Since(start))
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("failure log not observed")
	}
}
