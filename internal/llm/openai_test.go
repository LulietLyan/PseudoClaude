package llm

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"PseudoClaude/internal/tools"

	"github.com/openai/openai-go/v3"
)

func TestOpenAIToolDefinitionConversion(t *testing.T) {
	converted := toOpenAITools([]tools.Definition{testDefinition()})
	if len(converted) != 1 || converted[0].OfFunction == nil {
		t.Fatalf("converted = %+v", converted)
	}
	fn := converted[0].OfFunction.Function
	if fn.Name != "read_file" || !fn.Description.Valid() || fn.Description.Value != "read file" {
		t.Fatalf("function = %+v", fn)
	}
	if fn.Parameters["type"] != "object" {
		t.Fatalf("parameters = %+v", fn.Parameters)
	}
}

func TestOpenAIMessagesIncludeToolHistory(t *testing.T) {
	msgs := toOpenAIMessages(Request{
		System: System{Stable: "stable"},
		Messages: []Message{
			{Role: "assistant", ToolCalls: []ToolCall{{
				ID:        "call_1",
				Name:      "read_file",
				Arguments: json.RawMessage(`{"path":"README.md"}`),
			}}},
			{Role: "user", ToolResult: &ToolResult{
				CallID:  "call_1",
				Name:    "read_file",
				Content: `{"ok":true}`,
			}},
		},
	})
	if len(msgs) != 3 {
		t.Fatalf("message count = %d", len(msgs))
	}
	if msgs[1].OfAssistant == nil || len(msgs[1].OfAssistant.ToolCalls) != 1 {
		t.Fatalf("assistant = %+v", msgs[1])
	}
	call := msgs[1].OfAssistant.ToolCalls[0].OfFunction
	if call == nil || call.ID != "call_1" || call.Function.Name != "read_file" || call.Function.Arguments != `{"path":"README.md"}` {
		t.Fatalf("tool call = %+v", call)
	}
	if msgs[2].OfTool == nil || msgs[2].OfTool.ToolCallID != "call_1" {
		t.Fatalf("tool result = %+v", msgs[2])
	}
}

func TestOpenAIMessagesSystemEnvironmentAndReminder(t *testing.T) {
	msgs := toOpenAIMessages(Request{
		System:   System{Stable: "stable", Environment: "env"},
		Messages: []Message{{Role: "user", Content: "hello"}},
		Reminder: "<system-reminder>plan</system-reminder>",
	})
	if len(msgs) != 3 {
		t.Fatalf("message count = %d", len(msgs))
	}
	if msgs[0].OfSystem == nil || !strings.Contains(msgs[0].OfSystem.Content.OfString.Value, "stable\n\nenv") {
		t.Fatalf("system message = %+v", msgs[0])
	}
	if msgs[2].OfUser == nil || !strings.Contains(msgs[2].OfUser.Content.OfString.Value, "system-reminder") {
		t.Fatalf("reminder message = %+v", msgs[2])
	}
}

func TestOpenAIUsageMapsCachedTokens(t *testing.T) {
	got := openAIUsageFromCompletionUsage(openai.CompletionUsage{
		PromptTokens:     10,
		CompletionTokens: 20,
		TotalTokens:      30,
		PromptTokensDetails: openai.CompletionUsagePromptTokensDetails{
			CachedTokens: 7,
		},
	})
	if got.InputTokens != 10 || got.OutputTokens != 20 || got.TotalTokens != 30 || got.CacheWrite != 0 || got.CacheRead != 7 {
		t.Fatalf("usage = %+v", got)
	}
	if got := openAIUsageFromCompletionUsage(openai.CompletionUsage{}); got != nil {
		t.Fatalf("empty usage = %+v", got)
	}
}

func TestOpenAICompatibleEmptyJSONTail(t *testing.T) {
	if !isOpenAICompatibleEmptyJSONTail(errors.New("unexpected end of JSON input")) {
		t.Fatal("expected compatible empty JSON tail")
	}
	if !isOpenAICompatibleEmptyJSONTail(io.ErrUnexpectedEOF) {
		t.Fatal("expected unexpected EOF to match")
	}
	if isOpenAICompatibleEmptyJSONTail(errors.New("401 unauthorized")) {
		t.Fatal("auth error should not match")
	}
}

func TestOpenAIAccumulatorBuildsToolCallFromFragments(t *testing.T) {
	acc := openai.ChatCompletionAccumulator{}
	chunks := []openai.ChatCompletionChunk{
		openAIChunk(openai.ChatCompletionChunkChoiceDeltaToolCall{
			Index: 0,
			ID:    "call_1",
			Type:  "function",
			Function: openai.ChatCompletionChunkChoiceDeltaToolCallFunction{
				Name:      "read_file",
				Arguments: `{"path":`,
			},
		}),
		openAIChunk(openai.ChatCompletionChunkChoiceDeltaToolCall{
			Index: 0,
			Function: openai.ChatCompletionChunkChoiceDeltaToolCallFunction{
				Arguments: `"README.md"}`,
			},
		}),
		{
			ID: "chunk",
			Choices: []openai.ChatCompletionChunkChoice{{
				Index:        0,
				FinishReason: "tool_calls",
			}},
		},
	}
	var got *ToolCall
	for _, chunk := range chunks {
		acc.AddChunk(chunk)
		if toolCall, ok := acc.JustFinishedToolCall(); ok {
			got = &ToolCall{ID: toolCall.ID, Name: toolCall.Name, Arguments: json.RawMessage(toolCall.Arguments)}
		}
	}
	if got == nil {
		t.Fatal("expected finished tool call")
	}
	if got.ID != "call_1" || got.Name != "read_file" || string(got.Arguments) != `{"path":"README.md"}` {
		t.Fatalf("tool call = %+v", got)
	}
}

func openAIChunk(call openai.ChatCompletionChunkChoiceDeltaToolCall) openai.ChatCompletionChunk {
	data, err := json.Marshal(map[string]any{
		"id":      "chunk",
		"object":  "chat.completion.chunk",
		"created": 1,
		"model":   "test",
		"choices": []map[string]any{{
			"index": 0,
			"delta": map[string]any{
				"tool_calls": []any{map[string]any{
					"index": call.Index,
					"id":    call.ID,
					"type":  call.Type,
					"function": map[string]any{
						"name":      call.Function.Name,
						"arguments": call.Function.Arguments,
					},
				}},
			},
		}},
	})
	if err != nil {
		panic(err)
	}
	var chunk openai.ChatCompletionChunk
	if err := json.Unmarshal(data, &chunk); err != nil {
		panic(err)
	}
	return chunk
}
