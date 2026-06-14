package agent

import (
	"context"
	"strings"

	"PseudoClaude/internal/llm"
)

type streamCollector struct {
	text      strings.Builder
	toolCalls []llm.ToolCall
	usage     *llm.Usage
}

type roundOutput struct {
	Text      string
	ToolCalls []llm.ToolCall
	Usage     *llm.Usage
}

func (c *streamCollector) collect(ctx context.Context, iteration int, stream <-chan llm.StreamEvent, events chan<- Event) (roundOutput, error) {
	for {
		select {
		case <-ctx.Done():
			return roundOutput{}, ctx.Err()
		case event, ok := <-stream:
			if !ok {
				return c.output(), nil
			}
			if event.Err != nil {
				return roundOutput{}, event.Err
			}
			if event.Text != "" {
				c.text.WriteString(event.Text)
				if !sendEvent(ctx, events, Event{Type: EventTextDelta, Iteration: iteration, Text: event.Text}) {
					return roundOutput{}, ctx.Err()
				}
			}
			if event.ToolCall != nil {
				c.toolCalls = append(c.toolCalls, *event.ToolCall)
			}
			if event.Usage != nil {
				usage := *event.Usage
				c.usage = &usage
				if !sendEvent(ctx, events, Event{Type: EventUsage, Iteration: iteration, Usage: &usage}) {
					return roundOutput{}, ctx.Err()
				}
			}
			if event.Done {
				return c.output(), nil
			}
		}
	}
}

func (c *streamCollector) output() roundOutput {
	return roundOutput{
		Text:      c.text.String(),
		ToolCalls: append([]llm.ToolCall(nil), c.toolCalls...),
		Usage:     c.usage,
	}
}

func sendEvent(ctx context.Context, ch chan<- Event, event Event) bool {
	select {
	case <-ctx.Done():
		return false
	case ch <- event:
		return true
	}
}
