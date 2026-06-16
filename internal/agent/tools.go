package agent

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"PseudoClaude/internal/llm"
	"PseudoClaude/internal/permission"
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

func splitToolBatches(registry *tools.Registry, calls []llm.ToolCall, opts toolExecutionOptions, engine *permission.Engine, mode permission.Mode) []toolBatch {
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
		if shouldRunSerialForPermission(engine, mode, call, safety, known) || (known && !opts.allows(safety)) {
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

func executeToolCalls(ctx context.Context, registry *tools.Registry, env tools.Env, iteration int, calls []llm.ToolCall, events chan<- Event, opts toolExecutionOptions, engine *permission.Engine, mode permission.Mode) ([]ToolResult, error) {
	if registry == nil {
		registry, _ = tools.NewRegistry()
	}
	results := make([]ToolResult, len(calls))
	filled := make([]bool, len(calls))
	for _, batch := range splitToolBatches(registry, calls, opts, engine, mode) {
		if ctx.Err() != nil {
			return orderedResults(results, filled), ctx.Err()
		}
		batchMode := "serial"
		if batch.concurrent {
			batchMode = "concurrent"
		}
		sendEvent(ctx, events, Event{Type: EventProgress, Iteration: iteration, Message: "starting " + batchMode + " tool batch"})
		if batch.concurrent {
			var wg sync.WaitGroup
			var mu sync.Mutex
			for _, item := range batch.items {
				item := item
				wg.Add(1)
				go func() {
					defer wg.Done()
					result := executeOneTool(ctx, registry, env, iteration, item.call, events, opts, engine, mode)
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
				result := executeOneTool(ctx, registry, env, iteration, item.call, events, opts, engine, mode)
				results[item.index] = result
				filled[item.index] = true
			}
		}
		sendEvent(ctx, events, Event{Type: EventProgress, Iteration: iteration, Message: "finished tool batch"})
	}
	return orderedResults(results, filled), ctx.Err()
}

func executeOneTool(ctx context.Context, registry *tools.Registry, env tools.Env, iteration int, call llm.ToolCall, events chan<- Event, opts toolExecutionOptions, engine *permission.Engine, mode permission.Mode) ToolResult {
	started := time.Now()
	sendEvent(ctx, events, Event{Type: EventToolCallStart, Iteration: iteration, ToolCall: &call})
	result := permissionCheckedTool(ctx, registry, env, iteration, call, events, opts, engine, mode)
	out := ToolResult{Call: call, Result: result, Elapsed: time.Since(started)}
	sendEvent(ctx, events, Event{Type: EventToolResult, Iteration: iteration, ToolResult: &out})
	sendEvent(ctx, events, Event{Type: EventToolCallDone, Iteration: iteration, ToolCall: &call, ToolResult: &out})
	return out
}

func permissionCheckedTool(ctx context.Context, registry *tools.Registry, env tools.Env, iteration int, call llm.ToolCall, events chan<- Event, opts toolExecutionOptions, engine *permission.Engine, mode permission.Mode) tools.Result {
	if engine == nil {
		return executeAllowedTool(ctx, registry, env, call, opts)
	}
	safety, _ := registry.Safety(call.Name)
	check := engine.Check(mode, call, safety)
	switch check.Decision {
	case permission.DecisionAllow:
		return executeAllowedTool(ctx, registry, env, call, opts)
	case permission.DecisionDeny:
		return permissionDeniedResult(call, check)
	case permission.DecisionAsk:
		decision, err := requestApproval(ctx, call, check, events, iteration)
		if err != nil {
			return tools.Failure(call.Name, "permission_canceled", err.Error(), permissionMetadata(call, check))
		}
		switch decision {
		case permission.ApprovalAllowOnce:
			return executeAllowedTool(ctx, registry, env, call, opts)
		case permission.ApprovalAllowSession:
			if err := engine.AllowForSession(call); err != nil {
				return tools.Failure(call.Name, "permission_error", err.Error(), permissionMetadata(call, check))
			}
			return executeAllowedTool(ctx, registry, env, call, opts)
		case permission.ApprovalAllowForever:
			if err := engine.PersistLocalAllow(call); err != nil {
				return tools.Failure(call.Name, "permission_error", err.Error(), permissionMetadata(call, check))
			}
			return executeAllowedTool(ctx, registry, env, call, opts)
		default:
			check.Source = "user"
			check.Reason = "user denied this tool call"
			return permissionDeniedResult(call, check)
		}
	default:
		return permissionDeniedResult(call, permission.CheckResult{Decision: permission.DecisionDeny, Source: "unknown", Reason: "permission engine returned an unknown decision"})
	}
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

func shouldRunSerialForPermission(engine *permission.Engine, mode permission.Mode, call llm.ToolCall, safety tools.Safety, known bool) bool {
	if engine == nil {
		return false
	}
	if !known || safety != tools.SafetyReadOnly {
		return true
	}
	check := engine.Check(mode, call, safety)
	return check.Decision == permission.DecisionAsk
}

func requestApproval(ctx context.Context, call llm.ToolCall, result permission.CheckResult, events chan<- Event, iteration int) (permission.ApprovalDecision, error) {
	req := &ApprovalRequest{
		Call:    call,
		Summary: summarizeCall(call),
		Reason:  result.Reason,
		Result:  result,
		Respond: make(chan permission.ApprovalDecision, 1),
	}
	if !sendEvent(ctx, events, Event{Type: EventApproval, Iteration: iteration, ToolCall: &call, Approval: req}) {
		if ctx.Err() != nil {
			return permission.ApprovalDenyOnce, ctx.Err()
		}
		return permission.ApprovalDenyOnce, context.Canceled
	}
	select {
	case decision := <-req.Respond:
		return decision, nil
	case <-ctx.Done():
		return permission.ApprovalDenyOnce, ctx.Err()
	}
}

func permissionDeniedResult(call llm.ToolCall, check permission.CheckResult) tools.Result {
	return tools.Failure(call.Name, "permission_denied", check.Reason, permissionMetadata(call, check))
}

func permissionMetadata(call llm.ToolCall, check permission.CheckResult) map[string]any {
	metadata := map[string]any{
		"call_id":  call.ID,
		"source":   check.Source,
		"decision": string(check.Decision),
	}
	if check.Rule != "" {
		metadata["rule"] = check.Rule
	}
	if check.Category != "" {
		metadata["category"] = string(check.Category)
	}
	if check.Target != "" {
		metadata["target"] = check.Target
	}
	return metadata
}

func summarizeCall(call llm.ToolCall) string {
	switch call.Name {
	case "run_command":
		if command, ok := permission.CommandTextForDisplay(call); ok {
			return command
		}
	case "read_file", "write_file", "edit_file", "find_files", "search_code":
		if target, ok := permission.TargetForDisplay(call); ok {
			return target
		}
	}
	if len(call.Arguments) == 0 {
		return call.Name
	}
	raw := string(call.Arguments)
	if len(raw) > 160 {
		raw = raw[:157] + "..."
	}
	return raw
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
