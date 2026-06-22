package agent

import (
	"context"
	"encoding/json"
	"testing"

	"PseudoClaude/internal/conversation"
	"PseudoClaude/internal/llm"
	"PseudoClaude/internal/tools"
)

func TestRunToCompletionReturnsAssistantText(t *testing.T) {
	provider := &fakeProvider{streams: [][]llm.StreamEvent{{{Text: "done"}, {Usage: &llm.Usage{TotalTokens: 3}}, {Done: true}}}}
	conv := &conversation.Conversation{}
	result := Runner{Provider: provider, Registry: &tools.Registry{}, Sub: SubRunOptions{IsSubAgent: true}}.RunToCompletion(context.Background(), RunToCompletionInput{
		Request:  Request{Conversation: conv},
		TaskText: "task",
	})
	if result.Text != "done" || result.Stop.Reason != StopCompleted || result.Usage.TotalTokens != 3 {
		t.Fatalf("result = %#v", result)
	}
	msgs := conv.Messages()
	if len(msgs) != 2 || msgs[0].Content != "task" || msgs[1].Content != "done" {
		t.Fatalf("conversation = %#v", msgs)
	}
}

func TestRunToCompletionToolStatsAndIsolation(t *testing.T) {
	provider := &fakeProvider{streams: [][]llm.StreamEvent{
		{{ToolCall: &llm.ToolCall{ID: "call", Name: "read_file", Arguments: json.RawMessage(`{}`)}}, {Done: true}},
		{{Text: "finished"}, {Done: true}},
	}}
	registry, _ := tools.NewRegistry(scriptedTool{name: "read_file", safety: tools.SafetyReadOnly})
	parent := &conversation.Conversation{}
	parent.AddUser("parent")
	child := &conversation.Conversation{}
	result := Runner{Provider: provider, Registry: registry, Sub: SubRunOptions{IsSubAgent: true}}.RunToCompletion(context.Background(), RunToCompletionInput{
		Request:  Request{Conversation: child},
		TaskText: "child task",
	})
	if result.Text != "finished" || result.ToolCount != 1 || result.LastTool != "read_file" {
		t.Fatalf("result = %#v", result)
	}
	if parent.Len() != 1 {
		t.Fatalf("parent conversation changed: %#v", parent.Messages())
	}
}

func TestRunToCompletionMaxTurnsKeepsLastText(t *testing.T) {
	provider := &fakeProvider{streams: [][]llm.StreamEvent{
		{{Text: "partial"}, {ToolCall: &llm.ToolCall{ID: "call", Name: "read_file", Arguments: json.RawMessage(`{}`)}}, {Done: true}},
	}}
	registry, _ := tools.NewRegistry(scriptedTool{name: "read_file", safety: tools.SafetyReadOnly})
	result := Runner{
		Provider: provider,
		Registry: registry,
		Config:   Config{MaxIterations: 1},
		Sub:      SubRunOptions{IsSubAgent: true},
	}.RunToCompletion(context.Background(), RunToCompletionInput{
		Request:  Request{Conversation: &conversation.Conversation{}},
		TaskText: "task",
	})
	if result.Text != "partial" || result.Stop.Reason != StopMaxIterations {
		t.Fatalf("result = %#v", result)
	}
}
