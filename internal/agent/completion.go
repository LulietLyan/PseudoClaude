package agent

import (
	"context"

	"PseudoClaude/internal/llm"
)

type RunToCompletionInput struct {
	Request       Request
	TaskText      string
	Events        chan<- Event
	StopOnMaxTurn bool
}

type CompletionResult struct {
	Text      string
	Stop      Stop
	Usage     llm.Usage
	ToolCount int
	LastTool  string
}

func (r Runner) RunToCompletion(ctx context.Context, in RunToCompletionInput) CompletionResult {
	req := in.Request
	if in.TaskText != "" {
		req.UserText = in.TaskText
	}
	events := r.Run(ctx, req)
	var result CompletionResult
	for event := range events {
		if in.Events != nil {
			in.Events <- event
		}
		switch event.Type {
		case EventAssistantText:
			result.Text = event.Text
		case EventUsage:
			if event.Usage != nil {
				result.Usage.InputTokens += event.Usage.InputTokens
				result.Usage.OutputTokens += event.Usage.OutputTokens
				result.Usage.TotalTokens += event.Usage.TotalTokens
				result.Usage.CacheWrite += event.Usage.CacheWrite
				result.Usage.CacheRead += event.Usage.CacheRead
			}
		case EventToolCallDone:
			if event.ToolCall != nil {
				result.ToolCount++
				result.LastTool = event.ToolCall.Name
			}
		case EventStop:
			if event.Stop != nil {
				result.Stop = *event.Stop
			}
		}
	}
	return result
}
