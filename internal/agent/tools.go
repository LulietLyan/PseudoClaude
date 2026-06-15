package agent

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"PseudoClaude/internal/llm"
	"PseudoClaude/internal/tools"
)

type toolBatch struct {
	concurrent bool
	items      []indexedToolCall
}

type toolExecutionOptions struct {
	AllowedSafety map[tools.Safety]bool
}

type indexedToolCall struct {
	index int
	call  llm.ToolCall
}

func splitToolBatches(registry *tools.Registry, calls []llm.ToolCall, opts toolExecutionOptions) []toolBatch {
	var batches []toolBatch
	var readonly []indexedToolCall
	flushReadonly := func() {
		if len(readonly) == 0 {
			return
		}
		batches = append(batches, toolBatch{concurrent: true, items: readonly})
		readonly = nil
	}
	for i, call := range calls {
		safety, known := registry.Safety(call.Name)
		if known && !opts.allows(safety) {
			flushReadonly()
			batches = append(batches, toolBatch{items: []indexedToolCall{{index: i, call: call}}})
			continue
		}
		if known && safety == tools.SafetyReadOnly {
			readonly = append(readonly, indexedToolCall{index: i, call: call})
			continue
		}
		flushReadonly()
		batches = append(batches, toolBatch{items: []indexedToolCall{{index: i, call: call}}})
	}
	flushReadonly()
	return batches
}

func executeToolCalls(ctx context.Context, registry *tools.Registry, env tools.Env, iteration int, calls []llm.ToolCall, events chan<- Event, opts toolExecutionOptions) ([]ToolResult, error) {
	if registry == nil {
		registry, _ = tools.NewRegistry()
	}
	results := make([]ToolResult, len(calls))
	filled := make([]bool, len(calls))
	for _, batch := range splitToolBatches(registry, calls, opts) {
		if ctx.Err() != nil {
			return orderedResults(results, filled), ctx.Err()
		}
		mode := "serial"
		if batch.concurrent {
			mode = "concurrent"
		}
		sendEvent(ctx, events, Event{Type: EventProgress, Iteration: iteration, Message: "starting " + mode + " tool batch"})
		if batch.concurrent {
			var wg sync.WaitGroup
			var mu sync.Mutex
			for _, item := range batch.items {
				item := item
				wg.Add(1)
				go func() {
					defer wg.Done()
					result := executeOneTool(ctx, registry, env, iteration, item.call, events, opts)
					mu.Lock()
					results[item.index] = result
					filled[item.index] = true
					mu.Unlock()
				}()
			}
			wg.Wait()
		} else {
			for _, item := range batch.items {
				if ctx.Err() != nil {
					return orderedResults(results, filled), ctx.Err()
				}
				result := executeOneTool(ctx, registry, env, iteration, item.call, events, opts)
				results[item.index] = result
				filled[item.index] = true
			}
		}
		sendEvent(ctx, events, Event{Type: EventProgress, Iteration: iteration, Message: "finished tool batch"})
	}
	return orderedResults(results, filled), ctx.Err()
}

func executeOneTool(ctx context.Context, registry *tools.Registry, env tools.Env, iteration int, call llm.ToolCall, events chan<- Event, opts toolExecutionOptions) ToolResult {
	started := time.Now()
	sendEvent(ctx, events, Event{Type: EventToolCallStart, Iteration: iteration, ToolCall: &call})
	result := executeAllowedTool(ctx, registry, env, call, opts)
	out := ToolResult{Call: call, Result: result, Elapsed: time.Since(started)}
	sendEvent(ctx, events, Event{Type: EventToolResult, Iteration: iteration, ToolResult: &out})
	sendEvent(ctx, events, Event{Type: EventToolCallDone, Iteration: iteration, ToolCall: &call, ToolResult: &out})
	return out
}

func executeAllowedTool(ctx context.Context, registry *tools.Registry, env tools.Env, call llm.ToolCall, opts toolExecutionOptions) tools.Result {
	if safety, ok := registry.Safety(call.Name); ok && !opts.allows(safety) {
		return tools.Failure(call.Name, "tool_not_allowed", "tool is not available in the current mode", map[string]any{"call_id": call.ID, "safety": string(safety)})
	}
	return registry.Execute(ctx, tools.Call{
		ID:        call.ID,
		Name:      call.Name,
		Arguments: json.RawMessage(call.Arguments),
	}, env)
}

func (opts toolExecutionOptions) allows(safety tools.Safety) bool {
	if len(opts.AllowedSafety) == 0 {
		return true
	}
	return opts.AllowedSafety[safety]
}

func orderedResults(results []ToolResult, filled []bool) []ToolResult {
	out := make([]ToolResult, 0, len(results))
	for i := range results {
		if filled[i] {
			out = append(out, results[i])
		}
	}
	return out
}
