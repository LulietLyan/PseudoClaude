package compact

import "PseudoClaude/internal/llm"

func SelectRecent(messages []llm.Message) []llm.Message {
	if len(messages) == 0 {
		return nil
	}
	start := len(messages)
	var tokens int64
	for start > 0 {
		start--
		tokens += EstimateMessages(messages[start : start+1])
		if tokens >= RecentKeepTokens && len(messages)-start >= RecentKeepMessages {
			break
		}
	}
	start = ExpandToToolBoundary(messages, start)
	return cloneMessages(messages[start:])
}

func ExpandToToolBoundary(messages []llm.Message, start int) int {
	if start <= 0 || start >= len(messages) {
		if start < 0 {
			return 0
		}
		return start
	}
	if messages[start].ToolResult == nil {
		return start
	}
	callID := messages[start].ToolResult.CallID
	for i := start - 1; i >= 0; i-- {
		for _, call := range messages[i].ToolCalls {
			if call.ID == callID {
				return i
			}
		}
		if messages[i].ToolResult == nil && len(messages[i].ToolCalls) == 0 {
			break
		}
	}
	return start
}
