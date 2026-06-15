package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"PseudoClaude/internal/conversation"
	"PseudoClaude/internal/llm"
	"PseudoClaude/internal/tools"
)

type Runner struct {
	Provider llm.Provider
	Registry *tools.Registry
	Env      tools.Env
	Config   Config
}

type Config struct {
	MaxIterations       int
	MaxUnknownToolCalls int
}

type Request struct {
	Mode         Mode
	UserText     string
	PlanTask     string
	PlanText     string
	Conversation *conversation.Conversation
}

func DefaultConfig() Config {
	return Config{MaxIterations: 10, MaxUnknownToolCalls: 2}
}

func (c Config) normalize() Config {
	defaults := DefaultConfig()
	if c.MaxIterations <= 0 {
		c.MaxIterations = defaults.MaxIterations
	}
	if c.MaxUnknownToolCalls <= 0 {
		c.MaxUnknownToolCalls = defaults.MaxUnknownToolCalls
	}
	return c
}

func (r Runner) Run(ctx context.Context, req Request) <-chan Event {
	events := make(chan Event)
	go func() {
		defer close(events)
		r.run(ctx, req, events)
	}()
	return events
}

func (r Runner) run(ctx context.Context, req Request, events chan<- Event) {
	if ctx == nil {
		ctx = context.Background()
	}
	cfg := r.Config.normalize()
	if req.Conversation == nil {
		req.Conversation = &conversation.Conversation{}
	}
	if r.Registry == nil {
		r.Registry, _ = tools.NewRegistry()
	}
	if r.Provider == nil {
		err := errors.New("provider is nil")
		sendEvent(ctx, events, Event{Type: EventError, Err: err})
		sendStop(ctx, events, 0, StopStreamError, "provider is nil")
		return
	}

	userText, defs, toolOpts := r.prepareRequest(req)
	req.Conversation.AddUser(userText)
	sendEvent(ctx, events, Event{Type: EventProgress, Message: "agent run started"})

	unknownCount := 0
	for iteration := 1; ; iteration++ {
		if ctx.Err() != nil {
			sendStop(context.Background(), events, iteration-1, StopCanceled, "canceled")
			return
		}
		if iteration > cfg.MaxIterations {
			sendStop(ctx, events, iteration-1, StopMaxIterations, "reached maximum iterations")
			return
		}
		sendEvent(ctx, events, Event{Type: EventProgress, Iteration: iteration, Message: "requesting model"})

		collector := &streamCollector{}
		out, err := collector.collect(ctx, iteration, r.Provider.Stream(ctx, req.Conversation.Messages(), defs), events)
		if err != nil {
			if ctx.Err() != nil {
				sendStop(context.Background(), events, iteration, StopCanceled, "canceled")
				return
			}
			sendEvent(ctx, events, Event{Type: EventError, Iteration: iteration, Err: err})
			sendStop(ctx, events, iteration, StopStreamError, err.Error())
			return
		}

		if strings.TrimSpace(out.Text) != "" {
			req.Conversation.AddAssistant(out.Text)
			sendEvent(ctx, events, Event{Type: EventAssistantText, Iteration: iteration, Text: out.Text, Message: out.Text})
		}
		if len(out.ToolCalls) == 0 {
			unknownCount = 0
			sendStop(ctx, events, iteration, StopCompleted, "completed")
			return
		}

		req.Conversation.AddAssistantToolCalls(out.ToolCalls)
		results, err := executeToolCalls(ctx, r.Registry, r.Env, iteration, out.ToolCalls, events, toolOpts)
		for _, result := range results {
			req.Conversation.AddToolResult(llm.ToolResult{
				CallID:  result.Call.ID,
				Name:    result.Call.Name,
				Content: result.Result.JSON(),
				IsError: !result.Result.OK,
			})
			if r.Registry.IsKnown(result.Call.Name) {
				unknownCount = 0
			} else if result.Result.ErrorType == "unknown_tool" {
				unknownCount++
			}
		}
		if err != nil {
			sendStop(context.Background(), events, iteration, StopCanceled, "canceled")
			return
		}
		if unknownCount >= cfg.MaxUnknownToolCalls {
			sendStop(ctx, events, iteration, StopUnknownToolLimit, "too many unknown tool calls")
			return
		}
	}
}

func (r Runner) prepareRequest(req Request) (string, []tools.Definition, toolExecutionOptions) {
	if r.Registry == nil {
		return requestText(req), nil, toolExecutionOptions{}
	}
	switch req.Mode {
	case ModePlan:
		return planPrompt(req.PlanTask), r.Registry.DefinitionsBySafety(tools.SafetyReadOnly), toolExecutionOptions{
			AllowedSafety: map[tools.Safety]bool{tools.SafetyReadOnly: true},
		}
	case ModeDo:
		return doPrompt(req.PlanTask, req.PlanText), r.Registry.Definitions(), toolExecutionOptions{}
	default:
		return requestText(req), r.Registry.Definitions(), toolExecutionOptions{}
	}
}

func requestText(req Request) string {
	if strings.TrimSpace(req.UserText) != "" {
		return req.UserText
	}
	if strings.TrimSpace(req.PlanTask) != "" {
		return req.PlanTask
	}
	return strings.TrimSpace(req.PlanText)
}

func planPrompt(task string) string {
	return fmt.Sprintf(`Plan mode. Your job is to clarify and plan before implementation.

Rules:
- Use only read-only tools to inspect the workspace.
- Do not edit files, create directories, run commands, install dependencies, or make any project changes.
- If the task is broad or underspecified, ask concise clarifying questions first instead of inventing requirements.
- If you already have enough information, produce an implementation plan with target files, steps, validation, and risks.
- Do not claim that files or directories were created in Plan mode.

Task:
%s`, strings.TrimSpace(task))
}

func doPrompt(task, plan string) string {
	return fmt.Sprintf("Execution mode. Carry out the following task using the approved plan. Use the conversation history and tool results as context.\n\nOriginal task:\n%s\n\nPlan:\n%s", strings.TrimSpace(task), strings.TrimSpace(plan))
}

func sendStop(ctx context.Context, events chan<- Event, iteration int, reason StopReason, message string) {
	sendEvent(ctx, events, Event{
		Type:      EventStop,
		Iteration: iteration,
		Stop: &Stop{
			Reason:     reason,
			Message:    message,
			Iterations: iteration,
		},
	})
}
