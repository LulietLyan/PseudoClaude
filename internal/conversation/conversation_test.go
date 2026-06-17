package conversation

import (
	"encoding/json"
	"testing"

	"PseudoClaude/internal/llm"
)

func TestConversationMessagesPreserveOrderAndRoles(t *testing.T) {
	var c Conversation
	c.AddUser("hello")
	c.AddAssistant("hi")

	msgs := c.Messages()
	if len(msgs) != 2 {
		t.Fatalf("message count = %d, want 2", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "hello" {
		t.Fatalf("first message = %+v", msgs[0])
	}
	if msgs[1].Role != "assistant" || msgs[1].Content != "hi" {
		t.Fatalf("second message = %+v", msgs[1])
	}

	msgs[0].Content = "mutated"
	if c.Messages()[0].Content != "hello" {
		t.Fatal("Messages did not return a copy")
	}
}

func TestConversationToolHistoryAndDeepCopy(t *testing.T) {
	var c Conversation
	c.AddUser("use a tool")
	c.AddAssistantToolCall(llm.ToolCall{
		ID:        "call_1",
		Name:      "read_file",
		Arguments: json.RawMessage(`{"path":"README.md"}`),
	})
	c.AddToolResult(llm.ToolResult{
		CallID:  "call_1",
		Name:    "read_file",
		Content: `{"ok":true}`,
		IsError: false,
	})

	msgs := c.Messages()
	if len(msgs) != 3 {
		t.Fatalf("message count = %d, want 3", len(msgs))
	}
	if msgs[1].Role != "assistant" || len(msgs[1].ToolCalls) != 1 {
		t.Fatalf("tool call message = %+v", msgs[1])
	}
	if msgs[1].ToolCalls[0].ID != "call_1" || msgs[1].ToolCalls[0].Name != "read_file" || string(msgs[1].ToolCalls[0].Arguments) != `{"path":"README.md"}` {
		t.Fatalf("tool call = %+v", msgs[1].ToolCalls[0])
	}
	if msgs[2].Role != "user" || msgs[2].ToolResult == nil {
		t.Fatalf("tool result message = %+v", msgs[2])
	}
	if msgs[2].ToolResult.CallID != "call_1" || msgs[2].ToolResult.Content != `{"ok":true}` || msgs[2].ToolResult.IsError {
		t.Fatalf("tool result = %+v", msgs[2].ToolResult)
	}

	msgs[1].ToolCalls[0].Name = "mutated"
	msgs[2].ToolResult.Content = "mutated"
	next := c.Messages()
	if next[1].ToolCalls[0].Name != "read_file" {
		t.Fatal("tool call was not deep copied")
	}
	if next[2].ToolResult.Content != `{"ok":true}` {
		t.Fatal("tool result was not deep copied")
	}
}

func TestConversationMultipleToolCallsInOneAssistantMessage(t *testing.T) {
	var c Conversation
	calls := []llm.ToolCall{
		{ID: "call_1", Name: "read_file", Arguments: json.RawMessage(`{"path":"a"}`)},
		{ID: "call_2", Name: "search_code", Arguments: json.RawMessage(`{"pattern":"x"}`)},
	}
	c.AddAssistantToolCalls(calls)
	calls[0].Name = "mutated"

	msgs := c.Messages()
	if len(msgs) != 1 {
		t.Fatalf("message count = %d, want 1", len(msgs))
	}
	if msgs[0].Role != "assistant" || len(msgs[0].ToolCalls) != 2 {
		t.Fatalf("tool calls message = %+v", msgs[0])
	}
	if msgs[0].ToolCalls[0].ID != "call_1" || msgs[0].ToolCalls[1].ID != "call_2" {
		t.Fatalf("tool calls order = %+v", msgs[0].ToolCalls)
	}
	if msgs[0].ToolCalls[0].Name != "read_file" {
		t.Fatalf("tool call not copied: %+v", msgs[0].ToolCalls[0])
	}

	msgs[0].ToolCalls[1].Name = "mutated"
	next := c.Messages()
	if next[0].ToolCalls[1].Name != "search_code" {
		t.Fatal("multiple tool calls were not deep copied")
	}
}

func TestConversationReplaceMessagesDeepCopies(t *testing.T) {
	var c Conversation
	msgs := []llm.Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "call_1", Name: "read_file", Arguments: json.RawMessage(`{"path":"a"}`)}}},
		{Role: "user", ToolResult: &llm.ToolResult{CallID: "call_1", Name: "read_file", Content: "result"}},
	}
	c.ReplaceMessages(ReplaceReasonSnapshot, msgs)
	msgs[0].Content = "mutated"
	msgs[1].ToolCalls[0].Name = "mutated"
	msgs[2].ToolResult.Content = "mutated"

	got := c.Messages()
	if c.Len() != 3 {
		t.Fatalf("Len = %d", c.Len())
	}
	if got[0].Content != "hello" || got[1].ToolCalls[0].Name != "read_file" || got[2].ToolResult.Content != "result" {
		t.Fatalf("messages were not deep copied: %+v", got)
	}
	got[2].ToolResult.Content = "changed again"
	if c.Messages()[2].ToolResult.Content != "result" {
		t.Fatal("Messages did not deep copy after ReplaceMessages")
	}
}

func TestConversationHooks(t *testing.T) {
	var appended []llm.Message
	var replaceReason ReplaceReason
	var replaced []llm.Message
	c := New(Hooks{
		OnAppend: func(msg llm.Message) {
			appended = append(appended, msg)
		},
		OnReplace: func(reason ReplaceReason, messages []llm.Message) {
			replaceReason = reason
			replaced = messages
		},
	})

	c.AddUser("hello")
	c.ReplaceMessages(ReplaceReasonCompact, []llm.Message{{Role: "assistant", Content: "summary"}})

	if len(appended) != 1 || appended[0].Content != "hello" {
		t.Fatalf("append hook = %+v", appended)
	}
	if replaceReason != ReplaceReasonCompact || len(replaced) != 1 || replaced[0].Content != "summary" {
		t.Fatalf("replace hook reason=%s messages=%+v", replaceReason, replaced)
	}
	replaced[0].Content = "mutated"
	if c.Messages()[0].Content != "summary" {
		t.Fatal("replace hook did not receive a copy")
	}
}

func TestNewFromMessagesDeepCopies(t *testing.T) {
	source := []llm.Message{{Role: "user", Content: "hello"}}
	c := NewFromMessages(source, Hooks{})
	source[0].Content = "mutated"
	if c.Messages()[0].Content != "hello" {
		t.Fatal("NewFromMessages did not copy source")
	}
}
