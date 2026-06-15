package llm

import (
	"encoding/json"
	"strings"
	"testing"

	"PseudoClaude/internal/tools"

	"github.com/anthropics/anthropic-sdk-go"
)

func testDefinition() tools.Definition {
	return tools.Definition{
		Name:        "read_file",
		Description: "read file",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string"},
			},
			"required": []string{"path"},
		},
	}
}

func TestAnthropicToolDefinitionConversion(t *testing.T) {
	converted := toAnthropicTools([]tools.Definition{testDefinition()})
	if len(converted) != 1 || converted[0].OfTool == nil {
		t.Fatalf("converted = %+v", converted)
	}
	tool := converted[0].OfTool
	if tool.Name != "read_file" || !tool.Description.Valid() || tool.Description.Value != "read file" {
		t.Fatalf("tool = %+v", tool)
	}
	if len(tool.InputSchema.Required) != 1 || tool.InputSchema.Required[0] != "path" || tool.InputSchema.Properties == nil {
		t.Fatalf("schema = %+v", tool.InputSchema)
	}
}

func TestAnthropicSystemSeparatesStableAndEnvironment(t *testing.T) {
	system := toAnthropicSystem(System{Stable: "stable", Environment: "env"})
	if len(system) != 2 {
		t.Fatalf("system count = %d", len(system))
	}
	if system[0].Text != "stable" || system[0].CacheControl.TTL != "5m" {
		t.Fatalf("stable block = %+v", system[0])
	}
	if system[1].Text != "env" || system[1].CacheControl.TTL != "" {
		t.Fatalf("environment block = %+v", system[1])
	}
}

func TestAnthropicMessagesIncludeToolHistory(t *testing.T) {
	msgs := toAnthropicMessages([]Message{
		{Role: "assistant", ToolCalls: []ToolCall{{
			ID:        "call_1",
			Name:      "read_file",
			Arguments: json.RawMessage(`{"path":"README.md"}`),
		}}},
		{Role: "user", ToolResult: &ToolResult{
			CallID:  "call_1",
			Name:    "read_file",
			Content: `{"ok":true}`,
			IsError: false,
		}},
	})
	if len(msgs) != 2 {
		t.Fatalf("message count = %d", len(msgs))
	}
	if len(msgs[0].Content) != 1 || msgs[0].Content[0].OfToolUse == nil {
		t.Fatalf("assistant content = %+v", msgs[0].Content)
	}
	if msgs[0].Content[0].OfToolUse.ID != "call_1" || msgs[0].Content[0].OfToolUse.Name != "read_file" {
		t.Fatalf("tool use = %+v", msgs[0].Content[0].OfToolUse)
	}
	if len(msgs[1].Content) != 1 || msgs[1].Content[0].OfToolResult == nil {
		t.Fatalf("tool result content = %+v", msgs[1].Content)
	}
	if msgs[1].Content[0].OfToolResult.ToolUseID != "call_1" {
		t.Fatalf("tool result = %+v", msgs[1].Content[0].OfToolResult)
	}
}

func TestAnthropicReminderAppendsToUserOrCreatesUserMessage(t *testing.T) {
	msgs := toAnthropicMessages([]Message{{Role: "user", Content: "hello"}})
	withReminder := appendAnthropicReminder(msgs, "<system-reminder>plan</system-reminder>")
	if len(withReminder) != 1 || len(withReminder[0].Content) != 2 {
		t.Fatalf("reminder not appended to user: %+v", withReminder)
	}
	got := withReminder[0].Content[1].OfText.Text
	if !strings.Contains(got, "system-reminder") {
		t.Fatalf("reminder text = %q", got)
	}

	msgs = toAnthropicMessages([]Message{{Role: "assistant", Content: "hello"}})
	withReminder = appendAnthropicReminder(msgs, "reminder")
	if len(withReminder) != 2 || withReminder[1].Role != "user" {
		t.Fatalf("reminder did not create trailing user message: %+v", withReminder)
	}
}

func TestAnthropicUsageMapsCacheTokens(t *testing.T) {
	got := anthropicUsage(anthropic.Usage{
		InputTokens:              10,
		OutputTokens:             20,
		CacheCreationInputTokens: 30,
		CacheReadInputTokens:     40,
	})
	if got.InputTokens != 10 || got.OutputTokens != 20 || got.CacheWrite != 30 || got.CacheRead != 40 || got.TotalTokens != 100 {
		t.Fatalf("usage = %+v", got)
	}
}
