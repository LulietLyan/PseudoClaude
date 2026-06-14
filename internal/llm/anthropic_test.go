package llm

import (
	"encoding/json"
	"testing"

	"PseudoClaude/internal/tools"
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
