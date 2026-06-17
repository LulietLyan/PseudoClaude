package session

import (
	"bufio"
	"encoding/json"
	"os"
	"time"

	"PseudoClaude/internal/llm"
)

type LoadResult struct {
	ID          string
	Messages    []llm.Message
	LastMessage time.Time
	BadLines    int
	Truncated   bool
}

func Load(ctx Context) (LoadResult, error) {
	file, err := os.Open(ctx.JSONLPath)
	if err != nil {
		return LoadResult{}, err
	}
	defer file.Close()

	result := LoadResult{ID: ctx.ID}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		var entry Entry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			result.BadLines++
			continue
		}
		switch entry.Type {
		case EntryReplace:
			result.Messages = nil
		case EntryMessage, "":
			msg := messageFromEntry(entry)
			if msg.Role == "" {
				continue
			}
			result.Messages = append(result.Messages, msg)
			if entry.TS > 0 {
				result.LastMessage = time.Unix(entry.TS, 0)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return result, err
	}
	next, truncated := truncateDanglingToolCalls(result.Messages)
	result.Messages = next
	result.Truncated = truncated
	return result, nil
}

func messageFromEntry(entry Entry) llm.Message {
	msg := llm.Message{
		Role:      entry.Role,
		Content:   entry.Content,
		ToolCalls: append([]llm.ToolCall(nil), entry.ToolCalls...),
	}
	if entry.ToolResult != nil {
		result := *entry.ToolResult
		msg.ToolResult = &result
	}
	return msg
}

func truncateDanglingToolCalls(messages []llm.Message) ([]llm.Message, bool) {
	for i := len(messages) - 1; i >= 0; i-- {
		if len(messages[i].ToolCalls) == 0 {
			continue
		}
		if toolCallsSatisfied(messages, i) {
			return messages, false
		}
		return append([]llm.Message(nil), messages[:i]...), true
	}
	return messages, false
}

func toolCallsSatisfied(messages []llm.Message, idx int) bool {
	needed := make(map[string]bool)
	for _, call := range messages[idx].ToolCalls {
		needed[call.ID] = false
	}
	if len(needed) == 0 {
		return true
	}
	for _, msg := range messages[idx+1:] {
		if msg.ToolResult == nil {
			continue
		}
		if _, ok := needed[msg.ToolResult.CallID]; ok {
			needed[msg.ToolResult.CallID] = true
		}
	}
	for _, ok := range needed {
		if !ok {
			return false
		}
	}
	return true
}
