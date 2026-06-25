package agent

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"PseudoClaude/internal/llm"
)

func TestSummarizeCallTruncatesUTF8Safely(t *testing.T) {
	args := map[string]any{
		"team_name":   "demo",
		"assignee":    "alice",
		"title":       "总结 README.md 主要章节",
		"description": strings.Repeat("阅读当前工作区 README.md，并用中文总结主要章节。", 8),
	}
	data, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	summary := summarizeCall(llm.ToolCall{Name: "TaskCreate", Arguments: data})
	if !utf8.ValidString(summary) {
		t.Fatalf("summary is invalid UTF-8: %q", summary)
	}
	if strings.Contains(summary, "\uFFFD") {
		t.Fatalf("summary contains replacement char: %q", summary)
	}
	if !strings.HasSuffix(summary, "...") {
		t.Fatalf("summary should be truncated: %q", summary)
	}
}
