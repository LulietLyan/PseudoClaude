package compact

import (
	"encoding/json"
	"math"
	"strconv"

	"PseudoClaude/internal/llm"
)

func EstimateMessages(messages []llm.Message) int64 {
	if len(messages) == 0 {
		return 0
	}
	chars := messageChars(messages)
	if chars == 0 {
		return 0
	}
	return int64(math.Ceil(float64(chars) / EstimateCharsPerToken))
}

func EstimateWithAnchor(messages []llm.Message, anchor UsageAnchor) int64 {
	if anchor.Tokens <= 0 || anchor.MessageCount < 0 || anchor.MessageCount > len(messages) {
		return EstimateMessages(messages)
	}
	return anchor.Tokens + EstimateMessages(messages[anchor.MessageCount:])
}

func UsageTokens(usage *llm.Usage) (int64, bool) {
	if usage == nil {
		return 0, false
	}
	if usage.TotalTokens > 0 {
		return usage.TotalTokens, true
	}
	total := usage.InputTokens + usage.OutputTokens + usage.CacheRead + usage.CacheWrite
	if total <= 0 {
		return 0, false
	}
	return total, true
}

func messageChars(messages []llm.Message) int {
	total := 0
	for _, msg := range messages {
		total += len(msg.Role) + len(msg.Content)
		for _, call := range msg.ToolCalls {
			total += len(call.ID) + len(call.Name) + len(call.Arguments)
		}
		if msg.ToolResult != nil {
			total += len(msg.ToolResult.CallID)
			total += len(msg.ToolResult.Name)
			total += len(msg.ToolResult.Content)
			total += len(strconv.FormatBool(msg.ToolResult.IsError))
		}
	}
	return total
}

func serializeMessages(messages []llm.Message) string {
	data, err := json.Marshal(messages)
	if err != nil {
		return ""
	}
	return string(data)
}
